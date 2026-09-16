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

// minimumVMID is the lowest VMID Proxmox accepts.
const minimumVMID = 100

// DeleteVM implements the logic of destroying a VM.
func DeleteVM(ctx context.Context, machineScope *scope.MachineScope) error {
	vmID := machineScope.ProxmoxMachine.GetVirtualMachineID()
	node := machineScope.LocateProxmoxNode()

	// GetVirtualMachineID returns -1 for a machine that never got an ID.
	if vmID < minimumVMID {
		return releaseDeletedVM(machineScope)
	}

	ours, err := vmIsOurs(ctx, machineScope, node, vmID)
	if err != nil {
		return err
	}
	if !ours {
		return releaseDeletedVM(machineScope)
	}

	if _, err := machineScope.InfraCluster.ProxmoxClient.DeleteVM(ctx, node, vmID); err != nil {
		if VMNotFound(err) || errors.Is(err, goproxmox.ErrVMIDFree) {
			return releaseDeletedVM(machineScope)
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

// vmIsOurs reports whether the recorded VMID still holds this machine's VM.
// Proxmox hands a freed VMID to the next clone within seconds, so a stale
// record can point at another machine. An error means the state could not be
// read: the caller must never destroy a VM it failed to confirm.
func vmIsOurs(ctx context.Context, machineScope *scope.MachineScope, node string, vmID int64) (bool, error) {
	client := machineScope.InfraCluster.ProxmoxClient

	var found string
	vm, err := client.GetVM(ctx, node, vmID)
	if err == nil {
		found = vm.Name
	} else {
		// The recorded node holds no such VM. It is either gone, or it sits on
		// another node. Only a cluster-wide lookup tells the two apart.
		free, checkErr := client.CheckID(ctx, vmID)
		if checkErr != nil {
			return false, checkErr
		}
		if free {
			return false, nil
		}

		res, findErr := client.FindVMResource(ctx, uint64(vmID))
		if findErr != nil {
			return false, findErr
		}
		found = res.Name
	}

	if found != machineScope.ProxmoxMachine.GetName() {
		machineScope.Info("VMID belongs to another VM, skipping destroy", "vmID", vmID, "vmName", found)
		return false, nil
	}

	return true, nil
}

// releaseDeletedVM drops the machine from the cluster status and removes the
// finalizer, for a VM that is gone or was never ours.
func releaseDeletedVM(machineScope *scope.MachineScope) error {
	machineScope.InfraCluster.ProxmoxCluster.RemoveNodeLocation(machineScope.Name(), util.IsControlPlaneMachine(machineScope.Machine))
	ctrlutil.RemoveFinalizer(machineScope.ProxmoxMachine, infrav1.MachineFinalizer)
	return machineScope.InfraCluster.PatchObject()
}

// VMNotFound checks if the given err is related to that the VM is not found in Proxmox.
func VMNotFound(err error) bool {
	return strings.Contains(err.Error(), "does not exist")
}
