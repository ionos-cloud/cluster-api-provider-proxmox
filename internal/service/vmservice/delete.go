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

// DeleteVM implements the logic of destroying a VM.
func DeleteVM(ctx context.Context, machineScope *scope.MachineScope) error {
	vmID := machineScope.ProxmoxMachine.GetVirtualMachineID()
	node := machineScope.LocateProxmoxNode()

	// Proxmox reuses a freed VMID for the next clone within seconds, so the
	// recorded ID may now belong to another machine. Never destroy a VM whose
	// name is not ours; treat it as already gone.
	if vm, err := machineScope.InfraCluster.ProxmoxClient.GetVM(ctx, node, vmID); err == nil && vm.Name != machineScope.ProxmoxMachine.GetName() {
		machineScope.Info("VMID belongs to another VM, skipping destroy", "vmID", vmID, "vmName", vm.Name)
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
