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
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"
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

const (
	deletionTaskQMStop     = "qmstop"
	deletionTaskQMDestroy  = "qmdestroy"
	deletionTaskRetryAfter = time.Minute

	// Proxmox allocatable VMIDs start at 100; lower values include the unassigned sentinel
	// and Proxmox-reserved IDs that this provider should not try to delete.
	minimumProxmoxVMID int64 = 100
)

// DeleteVM implements the logic of destroying a VM.
func DeleteVM(ctx context.Context, machineScope *scope.MachineScope) error {
	if inFlight, err := reconcileInFlightDeletionTask(ctx, machineScope); err != nil || inFlight {
		return err
	}

	vmID := machineScope.ProxmoxMachine.GetVirtualMachineID()
	if unassignedOrReservedVMID(vmID) {
		return completeVMDeletion(machineScope)
	}
	node := machineScope.LocateProxmoxNode()

	task, err := machineScope.InfraCluster.ProxmoxClient.DeleteVM(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, goproxmox.ErrVMIDFree) {
			return completeVMDeletion(machineScope)
		}
		if VMNotFound(err) {
			verificationContext := fmt.Sprintf("VM lookup on node %q returned not found: %v", node, err)
			if completed, completionErr := completeIfVMIDFree(ctx, machineScope, verificationContext); completionErr != nil || completed {
				return completionErr
			}
			setDeletionFailedCondition(machineScope, fmt.Sprintf("VMID %d is still in use after VM lookup on node %q returned not found: %v", vmID, node, err))
			// reconcileDelete always returns DefaultReconcilerRequeue after nil, so keep the
			// finalizer and visible failure condition without switching to error backoff.
			return nil
		}
		setDeletionFailedCondition(machineScope, err.Error())
		return err
	}

	if task != nil {
		storeDeletionTask(machineScope, task)
	}

	return nil
}

func reconcileInFlightDeletionTask(ctx context.Context, machineScope *scope.MachineScope) (bool, error) {
	if machineScope.ProxmoxMachine.Status.TaskRef == nil {
		return false, nil
	}

	taskRef := *machineScope.ProxmoxMachine.Status.TaskRef
	task, err := taskservice.GetTask(ctx, machineScope)
	if err != nil {
		verificationContext := fmt.Sprintf("deletion task lookup %s failed: %v", taskRef, err)
		if completed, completionErr := completeIfVMIDFree(ctx, machineScope, verificationContext); completionErr != nil || completed {
			return completed, completionErr
		}
		if errors.Is(err, taskservice.ErrTaskNotFound) {
			clearTaskState(machineScope)
			return false, nil
		}
		setDeletingCondition(machineScope, fmt.Sprintf("waiting to retry deletion task lookup %s: %v", taskRef, err))
		return true, nil
	}
	if task == nil {
		return true, nil
	}
	if !isDeletionTask(task) {
		// Deletion owns cleanup from here; stale provisioning task refs (for example qmstart)
		// must not block finalizer progress even if the old task is still in flight.
		clearTaskState(machineScope)
		return false, nil
	}

	switch {
	case task.IsRunning:
		storeDeletionTask(machineScope, task)
		return true, nil
	case task.IsSuccessful && task.IsCompleted:
		if task.Type == deletionTaskQMDestroy {
			// qmdestroy success is authoritative for this deletion task. Do not gate
			// finalizer removal on CheckID because the VMID may already have been
			// reused by a replacement VM before this reconcile observes task completion.
			return true, completeVMDeletion(machineScope)
		}
		clearTaskState(machineScope)
		return false, nil
	case task.IsFailed:
		verificationContext := fmt.Sprintf("%s task %s failed: %s", task.Type, task.UPID, task.ExitStatus)
		if completed, err := completeIfVMIDFree(ctx, machineScope, verificationContext); err != nil || completed {
			return completed, err
		}
		if waitForRetryAfter(machineScope, task) {
			return true, nil
		}
		return false, nil
	default:
		return false, taskservice.NewRequeueError(fmt.Sprintf("unknown deletion task state %q for %q", task.ExitStatus, machineScope.ProxmoxMachine.Name), infrav1.DefaultReconcilerRequeue)
	}
}

func storeDeletionTask(machineScope *scope.MachineScope, task *proxmox.Task) {
	taskRef := string(task.UPID)
	machineScope.ProxmoxMachine.Status.TaskRef = &taskRef
	// A fresh/running deletion task supersedes any failed-task retry gate.
	machineScope.ProxmoxMachine.Status.RetryAfter = nil
	setDeletingCondition(machineScope, fmt.Sprintf("waiting for %s task %s", task.Type, task.UPID))
}

func clearTaskState(machineScope *scope.MachineScope) {
	machineScope.ProxmoxMachine.Status.TaskRef = nil
	machineScope.ProxmoxMachine.Status.RetryAfter = nil
}

func waitForRetryAfter(machineScope *scope.MachineScope, task *proxmox.Task) bool {
	retryAfter := machineScope.ProxmoxMachine.Status.RetryAfter
	if retryAfter == nil || retryAfter.IsZero() {
		setDeletionFailedCondition(machineScope, deletionTaskFailureMessage(task))
		machineScope.ProxmoxMachine.Status.RetryAfter = &metav1.Time{Time: time.Now().Add(deletionTaskRetryAfter)}
		return true
	}
	if time.Now().Before(retryAfter.Time) {
		setDeletionFailedCondition(machineScope, deletionTaskFailureMessage(task))
		return true
	}

	clearTaskState(machineScope)
	return false
}

func setDeletingCondition(machineScope *scope.MachineScope, message string) {
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status:  metav1.ConditionFalse,
		Reason:  infrav1.ProxmoxMachineVirtualMachineProvisionedDeletingReason,
		Message: message,
	})
}

func completeVMDeletion(machineScope *scope.MachineScope) error {
	clearTaskState(machineScope)
	// remove machine from cluster status
	machineScope.InfraCluster.ProxmoxCluster.RemoveNodeLocation(machineScope.Name(), util.IsControlPlaneMachine(machineScope.Machine))
	// The VM is deleted so remove the finalizer.
	ctrlutil.RemoveFinalizer(machineScope.ProxmoxMachine, infrav1.MachineFinalizer)
	return machineScope.InfraCluster.PatchObject()
}

func completeIfVMIDFree(ctx context.Context, machineScope *scope.MachineScope, verificationContext string) (bool, error) {
	vmID := machineScope.ProxmoxMachine.GetVirtualMachineID()
	if unassignedOrReservedVMID(vmID) {
		return true, completeVMDeletion(machineScope)
	}
	vmIDFree, err := machineScope.InfraCluster.ProxmoxClient.CheckID(ctx, vmID)
	if err != nil {
		setDeletingCondition(machineScope, checkIDErrorMessage(vmID, verificationContext, err))
		return false, checkIDError(vmID, verificationContext, err)
	}
	if !vmIDFree {
		return false, nil
	}
	return true, completeVMDeletion(machineScope)
}

func checkIDErrorMessage(vmID int64, verificationContext string, err error) string {
	if verificationContext == "" {
		return fmt.Sprintf("waiting to verify VMID %d is free: %v", vmID, err)
	}
	return fmt.Sprintf("waiting to verify VMID %d is free after %s: %v", vmID, verificationContext, err)
}

func checkIDError(vmID int64, verificationContext string, err error) error {
	if verificationContext == "" {
		return fmt.Errorf("verify VMID %d is free: %w", vmID, err)
	}
	return fmt.Errorf("verify VMID %d is free after %s: %w", vmID, verificationContext, err)
}

func setDeletionFailedCondition(machineScope *scope.MachineScope, message string) {
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status:  metav1.ConditionFalse,
		Reason:  infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason,
		Message: message,
	})
}

func deletionTaskFailureMessage(task *proxmox.Task) string {
	if task.ExitStatus == string(taskservice.TaskInfoStateOK) {
		return fmt.Sprintf("task %s failed but its exit status is OK; this should not happen", task.UPID)
	}
	return fmt.Sprintf("%s: %s", task.Type, task.ExitStatus)
}

func isDeletionTask(task *proxmox.Task) bool {
	return task.Type == deletionTaskQMStop || task.Type == deletionTaskQMDestroy
}

func unassignedOrReservedVMID(vmID int64) bool {
	return vmID < minimumProxmoxVMID
}

// VMNotFound checks if the given err is related to that the VM is not found in Proxmox.
func VMNotFound(err error) bool {
	return strings.Contains(err.Error(), "does not exist")
}
