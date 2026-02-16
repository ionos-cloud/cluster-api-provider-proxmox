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

package vmservice

import (
	"context"
	"fmt"

	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	"k8s.io/utils/ptr"
	capierrors "sigs.k8s.io/cluster-api/errors"

	// temporary replacement for "sigs.k8s.io/cluster-api/util" until v1beta2.
	"github.com/ionos-cloud/cluster-api-provider-proxmox/capiv1beta1/util"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

var (
	// ErrVMNotCreated VM is not created yet.
	ErrVMNotCreated = errors.New("vm not created")

	// ErrVMNotFound VM is not Found in Proxmox.
	ErrVMNotFound = errors.New("vm not found")

	// ErrVMNotInitialized VM is not Initialized in Proxmox.
	ErrVMNotInitialized = errors.New("vm not initialized")
)

// FindVM returns the Proxmox VM if the vmID is set, otherwise
// returns ErrVMNotCreated or ErrVMNotFound if the VM doesn't exist.
func FindVM(ctx context.Context, scope *scope.MachineScope) (*proxmox.VirtualMachine, error) {
	// find the vm
	vmID := scope.GetVirtualMachineID()
	if vmID > 0 {
		node := scope.LocateProxmoxNode()

		vm, err := scope.InfraCluster.ProxmoxClient.GetVM(ctx, node, vmID)
		if err != nil {
			scope.Error(err, "unable to find vm")
			return nil, ErrVMNotFound
		}
		if vm.Name != scope.ProxmoxMachine.GetName() {
			scope.Error(err, "vm is not initialized yet")
			return nil, ErrVMNotInitialized
		}
		return vm, nil
	}

	scope.Info("vmid doesn't exist yet")
	return nil, ErrVMNotCreated
}

func updateVMLocation(ctx context.Context, s *scope.MachineScope) error {
	// if there's an associated task, requeue.
	if s.ProxmoxMachine.Status.TaskRef != nil {
		return errors.New("cannot update with active task")
	}

	// The controller sometimes might to run into the issue, that a VM is orphaned,
	// because it failed to properly update the status.
	// In this case it should try to locate the orphaned VM inside the
	// Proxmox cluster and update the status accordingly.

	vmID := s.GetVirtualMachineID()

	// We are looking for a machine with the ID and check if the name matches.
	// Then we have to update the node in the machine and cluster status.
	rsc, err := s.InfraCluster.ProxmoxClient.FindVMResource(ctx, uint64(vmID))
	if err != nil {
		return err
	}

	// find the VM, to make sure the vm config is up-to-date.
	vm, err := s.InfraCluster.ProxmoxClient.GetVM(ctx, rsc.Node, vmID)
	if err != nil {
		return errors.Wrapf(err, "unable to find vm with id %d", rsc.VMID)
	}

	// Requeue if machine doesn't have a name yet.
	// It seems that the Proxmox source API does not always provide
	// the latest information about the resources in the cluster.
	// It might happen that even when a task is already finished,
	// we still have to wait until we can get the correct
	// information for a particular resource.
	if vm.VirtualMachineConfig.Name == "" {
		return errors.New("vm exists but does not have a name yet")
	}

	// If there is a machine with an ID that doesn't match name of the
	// Proxmox machine, we need to stop right there.
	machineName := s.ProxmoxMachine.GetName()
	if vm.VirtualMachineConfig.Name != machineName {
		err := fmt.Errorf("expected VM name to match %q but it was %q", vm.Name, machineName)
		s.SetFailureMessage(err)
		s.SetFailureReason(capierrors.MachineStatusError("UnkownMachine"))
		return err
	}

	// Update the Proxmox node in the status.
	s.ProxmoxMachine.Status.ProxmoxNode = ptr.To(vm.Node)

	// Attempt to update the cluster status
	updated := s.InfraCluster.ProxmoxCluster.UpdateNodeLocation(
		machineName,
		vm.Node,
		util.IsControlPlaneMachine(s.Machine),
	)

	if updated {
		return s.InfraCluster.PatchObject()
	}

	return nil
}
