/*
Copyright 2023-2026 IONOS Cloud.

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
	"slices"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"

	// temporary replacement for "sigs.k8s.io/cluster-api/util" until v1beta2.
	"github.com/ionos-cloud/cluster-api-provider-proxmox/capiv1beta1/util"

	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
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

	optionSockets     = "sockets"
	optionCores       = "cores"
	optionMemory      = "memory"
	optionTags        = "tags"
	optionDescription = "description"
)

// ErrNoVMIDInRangeFree is returned if no free VMID is found in the specified vmIDRange.
var ErrNoVMIDInRangeFree = errors.New("No free vmid found in vmIDRange")

// ReconcileVM makes sure that the VM is in the desired state by:
//  1. Creating the VM if it does not exist, then...
//  2. Updating the VM with the bootstrap data, such as the cloud-init meta and user data, before...
//  3. Powering on the VM, and finally...
//  4. Returning the real-time state of the VM to the caller
func ReconcileVM(ctx context.Context, scope *scope.MachineScope) (infrav1.VirtualMachine, error) {
	// Initialize the result.
	vm := infrav1.VirtualMachine{
		Name:  scope.Name(),
		State: infrav1.VirtualMachineStatePending,
	}

	// If there is an in-flight task associated with this VM then do not
	// reconcile the VM until the task is completed.
	if inFlight, err := taskservice.ReconcileInFlightTask(ctx, scope); err != nil || inFlight {
		return vm, err
	}
	scope.Logger.V(4).Info("proxmox machine state", "state", conditions.GetReason(scope.ProxmoxMachine, infrav1.VMProvisionedCondition))

	// TODO: This requires a proper state machine. We're reusing
	// the condition reasons in VMProvisionedConditions as a state machine
	// for convenience, but this definitely needs to be refactored.
	if requeue, err := ensureVirtualMachine(ctx, scope); err != nil || requeue {
		return vm, err
	} // VMProvisionedCondition reason is infrav1.CloningReason

	if requeue, err := reconcileVirtualMachineConfig(ctx, scope); err != nil || requeue {
		scope.Logger.V(4).Info("after reconcileVirtualMachineCOnfig", "machineName", scope.ProxmoxMachine.GetName(), "requeue", requeue, "err", err)
		return vm, err
	} // VMProvisionedCondition reason is infrav1.WaitingForDiskReconcilationReason

	if err := reconcileDisks(ctx, scope); err != nil {
		scope.Logger.V(4).Info("after reconcileDisks", "machineName", scope.ProxmoxMachine.GetName(), "err", err)
		return vm, err
	} // VMProvisionedCondition reason is infrav1.WaitingForStaticIPAllocationReason

	if requeue, err := reconcileIPAddresses(ctx, scope); err != nil || requeue {
		scope.Logger.V(4).Info("after reconcileIPAddresses", "machineName", scope.ProxmoxMachine.GetName(), "requeue", requeue, "err", err)
		return vm, err
	} // VMProvisionedCondition reason is infrav1.WaitingForBootstrapDataReason

	if requeue, err := reconcileBootstrapData(ctx, scope); err != nil || requeue {
		scope.Logger.V(4).Info("after reconcileBootstrapData", "machineName", scope.ProxmoxMachine.GetName(), "requeue", requeue, "err", err)
		return vm, err
	} // VMProvisionedCondition reason is infrav1.WaitingForVMPowerUpReason

	if requeue, err := reconcilePowerState(ctx, scope); err != nil || requeue {
		scope.Logger.V(4).Info("after reconcilePowerState", "machineName", scope.ProxmoxMachine.GetName(), "requeue", requeue, "err", err)
		return vm, err
	} // VMProvisionedCondition reason is infrav1.WaitingForClusterAPIMachineAddressesReason

	if err := reconcileMachineAddresses(scope); err != nil {
		scope.Logger.V(4).Info("after reconcileMachineAddresses", "machineName", scope.ProxmoxMachine.GetName(), "err", err)
		return vm, err
	} // VMProvisionedCondition reason is infrav1.WaitingForCloudInitReason

	if requeue, err := checkCloudInitStatus(ctx, scope); err != nil || requeue {
		scope.Logger.V(4).Info("after checkCloudInitStatus", "machineName", scope.ProxmoxMachine.GetName(), "requeue", requeue, "err", err)
		return vm, err
	} // VMProvisionedCondition reason is infrav1.WaitingForBootstrapReadyReason

	// handle invalid state of the machine
	if conditions.GetReason(scope.ProxmoxMachine, infrav1.VMProvisionedCondition) == infrav1.VMProvisionFailedReason {
		scope.Logger.V(4).Info("invalid proxmoxmachine state", "state", conditions.GetReason(scope.ProxmoxMachine, infrav1.VMProvisionedCondition))
		// If you end up here, please file a bug report.
		return vm, errors.New("invalid state (failed and no error)")
	}

	// if the root machine is ready, we can assume that the VM is ready as well.
	// unmount the cloud-init iso if it is still mounted.
	if scope.Machine.Status.BootstrapReady && scope.Machine.Status.NodeRef != nil {
		if err := unmountCloudInitISO(ctx, scope); err != nil {
			return vm, errors.Wrapf(err, "failed to unmount cloud-init iso for vm %s", scope.Name())
		}
	} // State Machine is finished
	scope.Logger.V(4).Info("condition", "condition", conditions.GetReason(scope.ProxmoxMachine, infrav1.VMProvisionedCondition))

	vm.State = infrav1.VirtualMachineStateReady
	return vm, nil
}

func checkCloudInitStatus(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) != infrav1.WaitingForCloudInitReason {
		// Machine is in the wrong state to reconcile, we only reconcile machines waiting for cloud init
		return false, nil
	}

	if !machineScope.SkipQemuGuestCheck() {
		if err := machineScope.InfraCluster.ProxmoxClient.QemuAgentStatus(ctx, machineScope.VirtualMachine); err != nil {
			return true, errors.Wrap(err, "error waiting for agent")
		}
	}

	// TODO: Is there a status for Ignition?
	if !machineScope.SkipCloudInitCheck() {
		if running, err := machineScope.InfraCluster.ProxmoxClient.CloudInitStatus(ctx, machineScope.VirtualMachine); err != nil || running {
			if running {
				return true, nil
			}
			if errors.Is(goproxmox.ErrCloudInitFailed, err) {
				conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.VMProvisionFailedReason, clusterv1.ConditionSeverityError, "%s", err)
				machineScope.SetFailureMessage(err)
				machineScope.SetFailureReason(capierrors.MachineStatusError("BootstrapFailed"))
			}
			return false, err
		}
	}

	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForBootstrapReadyReason, clusterv1.ConditionSeverityInfo, "")
	return false, nil
}

// ensureVirtualMachine creates a Proxmox VM if it doesn't exist and updates the given MachineScope.
func ensureVirtualMachine(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	// if there's an associated task, requeue.
	if machineScope.ProxmoxMachine.Status.TaskRef != nil {
		return true, nil
	}

	// Initialize the state machine for proxmox machine deployment.
	// NOTE: We are setting this condition only in case it does not exist, so we avoid to get flickering LastConditionTime
	// in case of cloning errors or powering on errors.
	if !conditions.Has(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) ||
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) == infrav1.WaitingForClusterInfrastructureReason ||
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) == infrav1.WaitingForBootstrapDataReason {
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.CloningReason, clusterv1.ConditionSeverityInfo, "")
	}

	// Before going further, we need the VM's managed object reference.
	vmRef, err := FindVM(ctx, machineScope)

	// TODO: Codeflow
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

		// Create the VM.
		resp, err := createVM(ctx, machineScope)
		if err != nil {
			conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.CloningFailedReason, clusterv1.ConditionSeverityWarning, "%s", err)
			return false, err
		}
		machineScope.Logger.V(4).Info("Task created", "taskID", resp.Task.ID)

		// make sure spec.VirtualMachineID is always set.
		machineScope.ProxmoxMachine.Status.TaskRef = ptr.To(string(resp.Task.UPID))
		machineScope.SetVirtualMachineID(resp.NewID)

		// requeue until cloning is finished
		return true, nil
	}

	// make sure spec.providerID is always set.
	biosUUID := extractUUID(vmRef.VirtualMachineConfig.SMBios1)
	machineScope.SetProviderID(biosUUID)

	// setting the VirtualMachine object for completing the reconciliation.
	machineScope.SetVirtualMachine(vmRef)

	// at this point the VM is found, so err must be nil
	return false, nil
}

func reconcileDisks(ctx context.Context, machineScope *scope.MachineScope) error {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) != infrav1.WaitingForDiskReconcilationReason {
		// Machine is in the wrong state to reconcile, we only reconcile Cloning VMs
		return nil
	}

	machineScope.V(4).Info("reconciling disks")
	disks := machineScope.ProxmoxMachine.Spec.Disks

	if disks != nil {
		vm := machineScope.VirtualMachine
		if vm.IsRunning() || ptr.Deref(machineScope.ProxmoxMachine.Status.Ready, false) {
			// We only want to do this before the machine was started or is ready
			return nil
		}

		if bv := disks.BootVolume; bv != nil {
			if _, err := machineScope.InfraCluster.ProxmoxClient.ResizeDisk(ctx, vm, bv.Disk, bv.FormatSize()); err != nil {
				machineScope.Error(err, "unable to set disk size", "vm", machineScope.VirtualMachine.VMID)
				return err
			}
		}
	}

	// Machine is now waiting for IPAddress Allocations, move State Machine along
	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForStaticIPAllocationReason, clusterv1.ConditionSeverityInfo, "")
	return nil
}

func reconcileVirtualMachineConfig(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) != infrav1.CloningReason {
		// Machine is in the wrong state to reconcile, we only reconcile Cloning VMs.
		return false, nil
	}

	if machineScope.VirtualMachine.IsRunning() || ptr.Deref(machineScope.ProxmoxMachine.Status.Ready, false) {
		// We only want to do this before the machine was started or is ready
		return false, nil
	}

	vmConfig := machineScope.VirtualMachine.VirtualMachineConfig

	// CPU & Memory
	var vmOptions []proxmox.VirtualMachineOption
	sockets := ptr.Deref(machineScope.ProxmoxMachine.Spec.NumSockets, 0)
	cores := ptr.Deref(machineScope.ProxmoxMachine.Spec.NumCores, 0)
	memory := ptr.Deref(machineScope.ProxmoxMachine.Spec.MemoryMiB, 0)
	if sockets > 0 && vmConfig.Sockets != int(sockets) {
		vmOptions = append(vmOptions, proxmox.VirtualMachineOption{Name: optionSockets, Value: sockets})
	}
	if cores > 0 && vmConfig.Cores != int(cores) {
		vmOptions = append(vmOptions, proxmox.VirtualMachineOption{Name: optionCores, Value: cores})
	}
	if memory > 0 && int(vmConfig.Memory) != int(memory) {
		vmOptions = append(vmOptions, proxmox.VirtualMachineOption{Name: optionMemory, Value: memory})
	}

	// Description
	if machineScope.ProxmoxMachine.Spec.Description != nil {
		if machineScope.VirtualMachine.VirtualMachineConfig.Description != *machineScope.ProxmoxMachine.Spec.Description {
			vmOptions = append(vmOptions, proxmox.VirtualMachineOption{Name: optionDescription, Value: machineScope.ProxmoxMachine.Spec.Description})
		}
	}

	// Network vmbrs.
	if machineScope.ProxmoxMachine.Spec.Network != nil && shouldUpdateNetworkDevices(machineScope) {
		// handing additional network devices.
		devices := machineScope.ProxmoxMachine.Spec.Network.NetworkDevices
		for _, v := range devices {
			vmOptions = append(vmOptions, proxmox.VirtualMachineOption{
				Name:  *v.Name,
				Value: formatNetworkDevice(*v.Model, *v.Bridge, v.MTU, v.VLAN),
			})
		}
	}

	// custom tags
	if machineScope.ProxmoxMachine.Spec.Tags != nil {
		machineScope.VirtualMachine.SplitTags()
		length := len(machineScope.VirtualMachine.VirtualMachineConfig.TagsSlice)
		for _, tag := range machineScope.ProxmoxMachine.Spec.Tags {
			if !machineScope.VirtualMachine.HasTag(tag) {
				machineScope.VirtualMachine.VirtualMachineConfig.TagsSlice = append(machineScope.VirtualMachine.VirtualMachineConfig.TagsSlice, tag)
			}
		}
		if len(machineScope.VirtualMachine.VirtualMachineConfig.TagsSlice) > length {
			vmOptions = append(vmOptions, proxmox.VirtualMachineOption{Name: optionTags, Value: strings.Join(machineScope.VirtualMachine.VirtualMachineConfig.TagsSlice, ";")})
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

	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForDiskReconcilationReason, clusterv1.ConditionSeverityInfo, "")
	return true, nil
}

func reconcileMachineAddresses(machineScope *scope.MachineScope) error {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) != infrav1.WaitingForClusterAPIMachineAddressesReason {
		// Machine is in the wrong state to reconcile, we only reconcile powered up VMs
		return nil
	}

	addr, err := getClusterAPIMachineAddresses(machineScope)
	if err != nil {
		machineScope.Error(err, "failed to retrieve machine addresses")
		return err
	}

	machineScope.SetAddresses(addr)

	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForCloudInitReason, clusterv1.ConditionSeverityInfo, "")
	return nil
}

func getClusterAPIMachineAddresses(scope *scope.MachineScope) ([]clusterv1.MachineAddress, error) {
	if !scope.VirtualMachine.IsRunning() {
		return nil, errors.New("unable to apply configuration as long as the virtual machine is not running")
	}

	addresses := []clusterv1.MachineAddress{
		{
			Type:    clusterv1.MachineHostName,
			Address: scope.Name(),
		},
	}

	machineAddresses := scope.ProxmoxMachine.GetIPAddresses()
	index := slices.IndexFunc(machineAddresses, func(s infrav1.IPAddressesSpec) bool {
		return s.NetName == "default"
	})
	// TODO: DHCP as InternalIP
	if index == -1 {
		return addresses, errors.Errorf("Machine has no default IPAddresses")
	}

	defaultAddresses := machineAddresses[index]

	for _, address := range slices.Concat(defaultAddresses.IPv4, defaultAddresses.IPv6) {
		if address == "" {
			continue
		}
		addresses = append(addresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineInternalIP,
			Address: address,
		})
	}

	return addresses, nil
}

func createVM(ctx context.Context, scope *scope.MachineScope) (proxmox.VMCloneResponse, error) {
	vmid, err := getVMID(ctx, scope)
	if err != nil {
		if errors.Is(err, ErrNoVMIDInRangeFree) {
			scope.SetFailureMessage(err)
			scope.SetFailureReason(capierrors.InsufficientResourcesMachineError)
		}
		return proxmox.VMCloneResponse{}, err
	}

	options := proxmox.VMCloneRequest{
		Node:  scope.ProxmoxMachine.GetNode(),
		NewID: int(vmid),
		Name:  scope.ProxmoxMachine.GetName(),
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
		scope.InfraCluster.ProxmoxCluster.Status.NodeLocations = new(infrav1.NodeLocations)
	}

	// if no target was specified but we have a set of nodes defined in the spec, we want to evenly distribute
	// the nodes across the cluster.
	if scope.ProxmoxMachine.Spec.Target == nil &&
		(len(scope.InfraCluster.ProxmoxCluster.Spec.AllowedNodes) > 0 || len(scope.ProxmoxMachine.Spec.AllowedNodes) > 0) {
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
	if templateID == -1 {
		var err error
		templateSelectorTags := scope.ProxmoxMachine.GetTemplateSelectorTags()
		options.Node, templateID, err = scope.InfraCluster.ProxmoxClient.FindVMTemplateByTags(ctx, templateSelectorTags)

		if err != nil {
			if errors.Is(err, goproxmox.ErrTemplateNotFound) {
				scope.SetFailureMessage(err)
				scope.SetFailureReason(capierrors.MachineStatusError("VMTemplateNotFound"))
				conditions.MarkFalse(scope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.VMProvisionFailedReason, clusterv1.ConditionSeverityError, "%s", err)
			}
			return proxmox.VMCloneResponse{}, err
		}
	}
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
	scope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: options.Name},
		Node:    node,
	}, util.IsControlPlaneMachine(scope.Machine))

	return res, scope.InfraCluster.PatchObject()
}

func getVMID(ctx context.Context, scope *scope.MachineScope) (int64, error) {
	if scope.ProxmoxMachine.Spec.VMIDRange != nil {
		vmIDRangeStart := scope.ProxmoxMachine.Spec.VMIDRange.Start
		vmIDRangeEnd := scope.ProxmoxMachine.Spec.VMIDRange.End
		if vmIDRangeStart != 0 && vmIDRangeEnd != 0 {
			return getNextFreeVMIDfromRange(ctx, scope, vmIDRangeStart, vmIDRangeEnd)
		}
	}
	// If VMIDRange is not defined, return 0 to let luthermonson/go-proxmox get the next free id.
	return 0, nil
}

func getNextFreeVMIDfromRange(ctx context.Context, scope *scope.MachineScope, vmIDRangeStart int64, vmIDRangeEnd int64) (int64, error) {
	usedVMIDs, err := getUsedVMIDs(ctx, scope)
	if err != nil {
		return 0, err
	}
	// Get next free vmid from the range
	for i := vmIDRangeStart; i <= vmIDRangeEnd; i++ {
		if slices.Contains(usedVMIDs, i) {
			continue
		}
		if vmidFree, err := scope.InfraCluster.ProxmoxClient.CheckID(ctx, i); err == nil && vmidFree {
			return i, nil
		} else if err != nil {
			return 0, err
		}
	}
	// Fail if we can't find a free vmid in the range.
	return 0, ErrNoVMIDInRangeFree
}

func getUsedVMIDs(ctx context.Context, scope *scope.MachineScope) ([]int64, error) {
	// Get all used vmids from existing ProxmoxMachines
	usedVMIDs := []int64{}
	proxmoxMachines, err := scope.InfraCluster.ListProxmoxMachinesForCluster(ctx)
	if err != nil {
		return usedVMIDs, err
	}
	for _, proxmoxMachine := range proxmoxMachines {
		if proxmoxMachine.GetVirtualMachineID() != -1 {
			usedVMIDs = append(usedVMIDs, proxmoxMachine.GetVirtualMachineID())
		}
	}
	return usedVMIDs, nil
}

var selectNextNode = scheduler.ScheduleVM

func unmountCloudInitISO(ctx context.Context, machineScope *scope.MachineScope) error {
	return machineScope.InfraCluster.ProxmoxClient.UnmountCloudInitISO(ctx, machineScope.VirtualMachine, inject.CloudInitISODevice)
}
