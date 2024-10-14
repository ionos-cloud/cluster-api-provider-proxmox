/*
Copyright 2023-2024 IONOS Cloud.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package vmservice implement Proxmox vm logic.
package vmservice

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1alpha2 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/scheduler"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/taskservice"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

const (
	// See the following link for a list of available config options:
	// https://pve.proxmox.com/pve-docs/api-viewer/index.html#/nodes/{node}/qemu/{vmid}/config

	optionSockets = "sockets"
	optionCores   = "cores"
	optionMemory  = "memory"
)

// ReconcileVM makes sure that the VM is in the desired state by:
//  1. Creating the VM if it does not exist, then...
//  2. Updating the VM with the bootstrap data, such as the cloud-init meta and user data, before...
//  3. Powering on the VM, and finally...
//  4. Returning the real-time state of the VM to the caller
func ReconcileVM(ctx context.Context, scope *scope.MachineScope) (infrav1alpha2.VirtualMachine, error) {
	// Initialize the result.
	vm := infrav1alpha2.VirtualMachine{
		Name:  scope.Name(),
		State: infrav1alpha2.VirtualMachineStatePending,
	}

	// If there is an in-flight task associated with this VM then do not
	// reconcile the VM until the task is completed.
	if inFlight, err := taskservice.ReconcileInFlightTask(ctx, scope); err != nil || inFlight {
		return vm, err
	}

	if requeue, err := ensureVirtualMachine(ctx, scope); err != nil || requeue {
		return vm, err
	}

	if requeue, err := reconcileVirtualMachineConfig(ctx, scope); err != nil || requeue {
		return vm, err
	}

	if err := reconcileDisks(ctx, scope); err != nil {
		return vm, err
	}

	if requeue, err := reconcileIPAddresses(ctx, scope); err != nil || requeue {
		return vm, err
	}

	if requeue, err := reconcileBootstrapData(ctx, scope); err != nil || requeue {
		return vm, err
	}

	if requeue, err := reconcilePowerState(ctx, scope); err != nil || requeue {
		return vm, err
	}

	if err := reconcileMachineAddresses(scope); err != nil {
		return vm, err
	}

	if requeue, err := checkCloudInitStatus(ctx, scope); err != nil || requeue {
		return vm, err
	}

	// if the root machine is ready, we can assume that the VM is ready as well.
	// unmount the cloud-init iso if it is still mounted.
	if scope.Machine.Status.BootstrapReady && scope.Machine.Status.NodeRef != nil {
		if err := unmountCloudInitISO(ctx, scope); err != nil {
			return vm, errors.Wrapf(err, "failed to unmount cloud-init iso for vm %s", scope.Name())
		}
	}

	vm.State = infrav1alpha2.VirtualMachineStateReady
	return vm, nil
}

func checkCloudInitStatus(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if !machineScope.VirtualMachine.IsRunning() {
		// skip if the vm is not running.
		return true, nil
	}

	if running, err := machineScope.InfraCluster.ProxmoxClient.CloudInitStatus(ctx, machineScope.VirtualMachine); err != nil || running {
		if running {
			return true, nil
		}
		if errors.Is(goproxmox.ErrCloudInitFailed, err) {
			conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition, infrav1alpha2.VMProvisionFailedReason, clusterv1.ConditionSeverityError, err.Error())
			machineScope.SetFailureMessage(err)
			machineScope.SetFailureReason(capierrors.MachineStatusError("BootstrapFailed"))
		}
		return false, err
	}

	return false, nil
}

// ensureVirtualMachine creates a Proxmox VM if it doesn't exist and updates the given MachineScope.
func ensureVirtualMachine(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	// if there's an associated task, requeue.
	if machineScope.ProxmoxMachine.Status.TaskRef != nil {
		return true, nil
	}
	// Before going further, we need the VM's managed object reference.
	vmRef, err := FindVM(ctx, machineScope)
	if err != nil {
		switch {
		case errors.Is(err, ErrVMNotFound):
			if err := updateVMLocation(ctx, machineScope); err != nil {
				return false, errors.Wrap(err, "error trying to locate vm")
			}

			// we always want to trigger reconciliation at this point.
			return false, err
		case errors.Is(err, ErrVMNotInitialized):
			return true, err
		case !errors.Is(err, ErrVMNotCreated):
			return false, err
		}

		// Otherwise, this is a new machine and the VM should be created.
		// NOTE: We are setting this condition only in case it does not exist, so we avoid to get flickering LastConditionTime
		// in case of cloning errors or powering on errors.
		if !conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition) {
			conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition, infrav1alpha2.CloningReason, clusterv1.ConditionSeverityInfo, "")
		}

		// Create the VM.
		resp, err := createVM(ctx, machineScope)
		if err != nil {
			conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition, infrav1alpha2.CloningFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
			return false, err
		}
		machineScope.Logger.V(4).Info("Task created", "taskID", resp.Task.ID)

		// make sure spec.VirtualMachineID is always set.
		machineScope.ProxmoxMachine.Status.TaskRef = ptr.To(string(resp.Task.UPID))
		machineScope.SetVirtualMachineID(resp.NewID)

		return true, nil
	}

	// make sure spec.providerID is always set.
	biosUUID := extractUUID(vmRef.VirtualMachineConfig.SMBios1)
	machineScope.SetProviderID(biosUUID)

	// setting the VirtualMachine object for completing the reconciliation.
	machineScope.SetVirtualMachine(vmRef)

	return false, nil
}

func reconcileDisks(ctx context.Context, machineScope *scope.MachineScope) error {
	machineScope.V(4).Info("reconciling disks")
	disks := machineScope.ProxmoxMachine.Spec.Disks
	if disks == nil {
		// nothing to do
		return nil
	}

	vm := machineScope.VirtualMachine
	if vm.IsRunning() || machineScope.ProxmoxMachine.Status.Ready {
		// We only want to do this before the machine was started or is ready
		return nil
	}

	if bv := disks.BootVolume; bv != nil {
		if err := machineScope.InfraCluster.ProxmoxClient.ResizeDisk(ctx, vm, bv.Disk, bv.FormatSize()); err != nil {
			machineScope.Error(err, "unable to set disk size", "vm", machineScope.VirtualMachine.VMID)
			return err
		}
	}

	return nil
}

func reconcileVirtualMachineConfig(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if machineScope.VirtualMachine.IsRunning() || machineScope.ProxmoxMachine.Status.Ready {
		// We only want to do this before the machine was started or is ready
		return false, nil
	}

	vmConfig := machineScope.VirtualMachine.VirtualMachineConfig

	// CPU & Memory
	var vmOptions []proxmox.VirtualMachineOption
	if value := machineScope.ProxmoxMachine.Spec.NumSockets; value > 0 && vmConfig.Sockets != int(value) {
		vmOptions = append(vmOptions, proxmox.VirtualMachineOption{Name: optionSockets, Value: value})
	}
	if value := machineScope.ProxmoxMachine.Spec.NumCores; value > 0 && vmConfig.Cores != int(value) {
		vmOptions = append(vmOptions, proxmox.VirtualMachineOption{Name: optionCores, Value: value})
	}
	if value := machineScope.ProxmoxMachine.Spec.MemoryMiB; value > 0 && int32(vmConfig.Memory) != value {
		vmOptions = append(vmOptions, proxmox.VirtualMachineOption{Name: optionMemory, Value: value})
	}

	// Network vmbrs.
	if machineScope.ProxmoxMachine.Spec.Network != nil && shouldUpdateNetworkDevices(machineScope) {
		// TODO: Remove
		/*
			// adding the default network device.
			vmOptions = append(vmOptions, proxmox.VirtualMachineOption{
				Name: infrav1alpha2.DefaultNetworkDevice,
				Value: formatNetworkDevice(
					*machineScope.ProxmoxMachine.Spec.Network.Default.Model,
					machineScope.ProxmoxMachine.Spec.Network.Default.Bridge,
					machineScope.ProxmoxMachine.Spec.Network.Default.MTU,
					machineScope.ProxmoxMachine.Spec.Network.Default.VLAN,
				),
			})
		*/

		// handing additional network devices.
		devices := machineScope.ProxmoxMachine.Spec.Network.NetworkDevices
		for _, v := range devices {
			vmOptions = append(vmOptions, proxmox.VirtualMachineOption{
				Name:  v.Name,
				Value: formatNetworkDevice(*v.Model, v.Bridge, v.MTU, v.VLAN),
			})
		}
	}

	if len(vmOptions) == 0 {
		return false, nil
	}

	machineScope.V(4).Info("reconciling virtual machine config")

	task, err := machineScope.InfraCluster.ProxmoxClient.ConfigureVM(ctx, machineScope.VirtualMachine, vmOptions...)
	if err != nil {
		return false, errors.Wrapf(err, "failed to configure VM %s", machineScope.Name())
	}

	machineScope.ProxmoxMachine.Status.TaskRef = ptr.To(string(task.UPID))
	return true, nil
}

func reconcileMachineAddresses(scope *scope.MachineScope) error {
	addr, err := getMachineAddresses(scope)
	if err != nil {
		scope.Error(err, "failed to retrieve machine addresses")
		return err
	}

	scope.SetAddresses(addr)
	return nil
}

func getMachineAddresses(scope *scope.MachineScope) ([]clusterv1.MachineAddress, error) {
	if !machineHasIPAddress(scope.ProxmoxMachine) {
		return nil, errors.New("machine does not yet have an ip address")
	}

	if !scope.VirtualMachine.IsRunning() {
		return nil, errors.New("unable to apply configuration as long as the virtual machine is not running")
	}

	addresses := []clusterv1.MachineAddress{
		{
			Type:    clusterv1.MachineHostName,
			Address: scope.Name(),
		},
	}

	if scope.InfraCluster.ProxmoxCluster.Spec.IPv4Config != nil {
		addresses = append(addresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineInternalIP,
			Address: scope.ProxmoxMachine.Status.IPAddresses[infrav1alpha2.DefaultNetworkDevice].IPV4[0], // TODO: Unfuck this
		})
	}

	if scope.InfraCluster.ProxmoxCluster.Spec.IPv6Config != nil {
		addresses = append(addresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineInternalIP,
			Address: scope.ProxmoxMachine.Status.IPAddresses[infrav1alpha2.DefaultNetworkDevice].IPV6[0], // TODO: Unfuck this
		})
	}

	return addresses, nil
}

func createVM(ctx context.Context, scope *scope.MachineScope) (proxmox.VMCloneResponse, error) {
	options := proxmox.VMCloneRequest{
		Node: scope.ProxmoxMachine.GetNode(),
		// NewID:       0, no need to provide newID
		Name: scope.ProxmoxMachine.GetName(),
	}

	if scope.ProxmoxMachine.Spec.Description != nil {
		options.Description = *scope.ProxmoxMachine.Spec.Description
	}
	if scope.ProxmoxMachine.Spec.Format != nil {
		options.Format = string(*scope.ProxmoxMachine.Spec.Format)
	}
	if scope.ProxmoxMachine.Spec.Full != nil {
		var full uint8
		if *scope.ProxmoxMachine.Spec.Full {
			full = 1
		}
		options.Full = full
	}
	if scope.ProxmoxMachine.Spec.Pool != nil {
		options.Pool = *scope.ProxmoxMachine.Spec.Pool
	}
	if scope.ProxmoxMachine.Spec.SnapName != nil {
		options.SnapName = *scope.ProxmoxMachine.Spec.SnapName
	}
	if scope.ProxmoxMachine.Spec.Storage != nil {
		options.Storage = *scope.ProxmoxMachine.Spec.Storage
	}
	if scope.ProxmoxMachine.Spec.Target != nil {
		options.Target = *scope.ProxmoxMachine.Spec.Target
	}

	if scope.InfraCluster.ProxmoxCluster.Status.NodeLocations == nil {
		scope.InfraCluster.ProxmoxCluster.Status.NodeLocations = new(infrav1alpha2.NodeLocations)
	}

	// if no target was specified but we have a set of nodes defined in the cluster spec, we want to evenly distribute
	// the nodes across the cluster.
	if scope.ProxmoxMachine.Spec.Target == nil && len(scope.InfraCluster.ProxmoxCluster.Spec.AllowedNodes) > 0 {
		// select next node as a target
		var err error
		options.Target, err = selectNextNode(ctx, scope)
		if err != nil {
			if errors.As(err, &scheduler.InsufficientMemoryError{}) {
				scope.SetFailureMessage(err)
				scope.SetFailureReason(capierrors.InsufficientResourcesMachineError)
			}
			return proxmox.VMCloneResponse{}, err
		}
	}

	templateID := scope.ProxmoxMachine.GetTemplateID()
	res, err := scope.InfraCluster.ProxmoxClient.CloneVM(ctx, int(templateID), options)
	if err != nil {
		return res, err
	}

	node := options.Target
	if node == "" {
		node = options.Node
	}

	scope.ProxmoxMachine.Status.ProxmoxNode = ptr.To(node)

	// if the creation was successful, we store the information about the node in the
	// cluster status
	scope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1alpha2.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: options.Name},
		Node:    node,
	}, util.IsControlPlaneMachine(scope.Machine))

	return res, scope.InfraCluster.PatchObject()
}

var selectNextNode = scheduler.ScheduleVM

func unmountCloudInitISO(ctx context.Context, machineScope *scope.MachineScope) error {
	return machineScope.InfraCluster.ProxmoxClient.UnmountCloudInitISO(ctx, machineScope.VirtualMachine, inject.CloudInitISODevice)
}
