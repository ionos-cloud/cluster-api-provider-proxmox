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
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

// errVMNotOwned means the recorded VMID is free or holds another machine's VM.
var errVMNotOwned = errors.New("vmid is not held by this machine's vm")

// DeleteVM implements the logic of destroying a VM.
func DeleteVM(ctx context.Context, machineScope *scope.MachineScope) error {
	vmID := machineScope.ProxmoxMachine.GetVirtualMachineID()

	// GetVirtualMachineID returns -1 for a machine that never got an ID.
	if vmID == -1 {
		return releaseMachine(machineScope)
	}

	node, err := locateOwnedVM(ctx, machineScope, vmID)
	switch {
	case errors.Is(err, errVMNotOwned):
		return releaseMachine(machineScope)
	case err != nil:
		return err
	}

	if _, err := machineScope.InfraCluster.ProxmoxClient.DeleteVM(ctx, node, vmID); err != nil {
		if VMNotFound(err) || errors.Is(err, goproxmox.ErrVMIDFree) {
			return releaseMachine(machineScope)
		}
		conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
			Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason,
		})
		return err
	}

	return nil
}

// locateOwnedVM returns the node that hosts the machine's VM.
// Proxmox hands a freed VMID to the next clone within seconds, so a stale
// record can point at another machine's VM. It returns errVMNotOwned if vmID
// is free or holds another VM or a container, and ErrVMNotInitialized while a
// clone is in progress. Any other error means the state could not be read:
// the caller must never destroy a VM it failed to confirm.
func locateOwnedVM(ctx context.Context, machineScope *scope.MachineScope, vmID int64) (string, error) {
	proxmoxClient := machineScope.InfraCluster.ProxmoxClient

	free, err := proxmoxClient.CheckID(ctx, vmID)
	if err != nil {
		return "", err
	}
	if free {
		return "", errVMNotOwned
	}

	res, err := proxmoxClient.FindVMResource(ctx, uint64(vmID))
	if err != nil {
		return "", err
	}

	// LXC containers share the VMID space, and GetVM reads only qemu VMs.
	if res.Type != "qemu" {
		machineScope.Info("VMID belongs to a container, skipping destroy", "vmID", vmID, "type", res.Type)
		return "", errVMNotOwned
	}

	// /cluster/resources takes the name from RRD data that pvestatd refreshes
	// every 10 s, so it can still show the previous holder of a reused VMID.
	// The VM config is current.
	vm, err := proxmoxClient.GetVM(ctx, res.Node, vmID)
	if err != nil {
		return "", err
	}

	switch vm.Name {
	case machineScope.ProxmoxMachine.GetName():
		return res.Node, nil
	case "", placeholderVMName(vmID):
		return "", ErrVMNotInitialized
	default:
		machineScope.Info("VMID belongs to another VM, skipping destroy", "vmID", vmID, "vmName", vm.Name)
		return "", errVMNotOwned
	}
}

// releaseMachine drops the machine from the cluster status and removes the
// finalizer, for a VM that is gone or was never ours.
func releaseMachine(machineScope *scope.MachineScope) error {
	machineScope.InfraCluster.ProxmoxCluster.RemoveNodeLocation(machineScope.Name(), util.IsControlPlaneMachine(machineScope.Machine))
	ctrlutil.RemoveFinalizer(machineScope.ProxmoxMachine, infrav1.MachineFinalizer)
	return machineScope.InfraCluster.PatchObject()
}

// VMNotFound checks if the given err is related to that the VM is not found in Proxmox.
func VMNotFound(err error) bool {
	return strings.Contains(err.Error(), "does not exist")
}
