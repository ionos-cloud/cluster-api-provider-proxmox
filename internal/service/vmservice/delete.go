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
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/taskservice"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

// DeleteVM implements the logic of destroying a VM.
//
// It returns true when a Proxmox task is in flight and the caller should
// requeue rather than act further.
func DeleteVM(ctx context.Context, machineScope *scope.MachineScope) (bool, error) {
	vmID := machineScope.ProxmoxMachine.GetVirtualMachineID()
	node := machineScope.LocateProxmoxNode()

	// Destroying a VM takes far longer than a reconcile interval, and the
	// deletion path is re-entered on every Machine and ProxmoxMachine event.
	// Without these checks each pass issues another qmstop/qmdestroy for a VM
	// that is already stopping or being destroyed, which is what fills the
	// Proxmox task log during a rolling update (#96).
	for _, taskType := range []string{goproxmox.TaskTypeDestroyVM, goproxmox.TaskTypeStopVM} {
		adopted, err := taskservice.AdoptActiveTask(ctx, machineScope, taskType)
		if err != nil {
			return false, err
		}
		if adopted {
			return true, nil
		}
	}

	task, err := machineScope.InfraCluster.ProxmoxClient.DeleteVM(ctx, node, vmID)
	if err != nil {
		if VMNotFound(err) || errors.Is(err, goproxmox.ErrVMIDFree) {
			// remove machine from cluster status
			machineScope.InfraCluster.ProxmoxCluster.RemoveNodeLocation(machineScope.Name(), util.IsControlPlaneMachine(machineScope.Machine))
			// The VM is deleted so remove the finalizer.
			ctrlutil.RemoveFinalizer(machineScope.ProxmoxMachine, infrav1.MachineFinalizer)
			return false, machineScope.InfraCluster.PatchObject()
		}
		conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
			Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason,
		})
		return false, err
	}

	// Track the destroy task so subsequent passes wait for it instead of
	// issuing another one.
	if task != nil {
		machineScope.ProxmoxMachine.Status.TaskRef = new(string(task.UPID))
		return true, nil
	}

	return false, nil
}

// VMNotFound checks if the given err is related to that the VM is not found in Proxmox.
func VMNotFound(err error) bool {
	return strings.Contains(err.Error(), "does not exist")
}
