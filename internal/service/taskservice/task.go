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

// Package taskservice implement logic related to Proxmox Task.
package taskservice

import (
	"context"
	"fmt"
	"time"

	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

// TaskInfoState the state of the task.
type TaskInfoState string

// all the task states.
const (
	TaskInfoStateQueued     = TaskInfoState("queued")
	TaskInfoStateRunning    = TaskInfoState("running")
	TaskInfoStateUnexpected = TaskInfoState("unexpected status")
	TaskInfoStateSuccess    = TaskInfoState("success")
	TaskInfoStateOK         = TaskInfoState("OK")
	TaskInfoStateError      = TaskInfoState("error")
)

var (
	// ErrTaskNotFound task is not found.
	ErrTaskNotFound = errors.New("task not found")
)

// GetTask returns the task relative to the current action.
func GetTask(ctx context.Context, machineScope *scope.MachineScope) (*proxmox.Task, error) {
	if machineScope.ProxmoxMachine.Status.TaskRef == nil {
		return nil, nil
	}

	task, err := machineScope.InfraCluster.ProxmoxClient.GetTask(ctx, *machineScope.ProxmoxMachine.Status.TaskRef)
	if err != nil {
		return nil, ErrTaskNotFound
	}

	return task, nil
}

// AdoptActiveTask makes a task-issuing operation idempotent against Proxmox.
//
// Status.TaskRef is read through the controller-runtime cache, so a reconcile
// that runs before a previous pass's status write has propagated sees no task
// ref and happily issues a second, duplicate task. Asking Proxmox directly is
// the only reliable answer to "did I already start this?".
//
// If a task of taskType is still running for this VM, its UPID is adopted into
// Status.TaskRef and true is returned, telling the caller to requeue instead of
// issuing another one.
func AdoptActiveTask(ctx context.Context, machineScope *scope.MachineScope, taskType string) (bool, error) {
	vmID := machineScope.ProxmoxMachine.GetVirtualMachineID()
	if vmID < 100 {
		return false, nil
	}

	task, err := machineScope.InfraCluster.ProxmoxClient.GetVMActiveTask(
		ctx, machineScope.LocateProxmoxNode(), vmID, taskType)
	if err != nil {
		return false, err
	}
	if task == nil {
		return false, nil
	}

	machineScope.Logger.Info("adopting in-flight Proxmox task instead of issuing a new one",
		"taskType", taskType, "task", string(task.UPID))
	machineScope.ProxmoxMachine.Status.TaskRef = new(string(task.UPID))

	return true, nil
}

// InFlight reports whether the task referenced by Status.TaskRef is still
// running, clearing the ref once it has completed.
//
// Unlike ReconcileInFlightTask it never touches the provisioning conditions or
// the RetryAfter backoff, so it is safe to use on the deletion path, which owns
// its own condition and must not be pushed back into the provisioning state
// machine.
func InFlight(ctx context.Context, machineScope *scope.MachineScope) (bool, error) {
	if machineScope.ProxmoxMachine.Status.TaskRef == nil {
		return false, nil
	}

	// GetTask collapses every lookup failure into ErrTaskNotFound, so an error
	// here only ever means Proxmox no longer knows this task. Drop the ref so
	// the caller can make progress rather than requeueing on it forever.
	task, err := GetTask(ctx, machineScope)
	if err != nil {
		machineScope.Logger.V(4).Info("dropping unknown task ref",
			"task", *machineScope.ProxmoxMachine.Status.TaskRef)
		machineScope.ProxmoxMachine.Status.TaskRef = nil

		// Intentionally not propagated: a task Proxmox has forgotten is not a
		// failure, and returning the error here would requeue on it forever.
		return false, nil //nolint:nilerr
	}
	if task == nil {
		machineScope.ProxmoxMachine.Status.TaskRef = nil
		return false, nil
	}

	if task.IsRunning {
		machineScope.Logger.V(4).Info("waiting for in-flight task",
			"taskType", task.Type, "task", string(task.UPID))
		return true, nil
	}

	machineScope.ProxmoxMachine.Status.TaskRef = nil

	return false, nil
}

// ReconcileInFlightTask determines if a task associated to the Proxmox VM object is in flight or not.
func ReconcileInFlightTask(ctx context.Context, machineScope *scope.MachineScope) (bool, error) {
	// skip if taskRef is nil.
	if machineScope.ProxmoxMachine.Status.TaskRef == nil {
		return false, nil
	}

	// Check to see if there is an in-flight task.
	t, err := GetTask(ctx, machineScope)
	if err != nil {
		return false, err
	}
	machineScope.Logger.V(4).Info("reconciling task", "task", t)

	return checkAndRetryTask(machineScope, t)
}

// checkAndRetryTask verifies whether the task exists and if the task should be reconciled.
// This is determined by the task state retryAfter value set.
func checkAndRetryTask(scope *scope.MachineScope, task *proxmox.Task) (bool, error) {
	// Make sure to requeue if no task was found.
	if task == nil {
		scope.Logger.V(4).Info("task is nil, requeueing")
		return true, nil
	}

	// Since RetryAfter is set, the last task failed. Wait for the RetryAfter time duration to expire
	// before checking/resetting the task.
	if !scope.ProxmoxMachine.Status.RetryAfter.IsZero() && time.Now().Before(scope.ProxmoxMachine.Status.RetryAfter.Time) {
		return false, NewRequeueError("last task failed", time.Until(scope.ProxmoxMachine.Status.RetryAfter.Time))
	}

	// Otherwise the course of action is determined by the state of the task.
	logger := scope.Logger.WithValues("taskType", task.Type)
	logger.Info("task found", "state", task.Status, "description", task.Type)

	switch {
	case task.IsRunning:
		logger.Info("task is still pending", "description", task.Type)
		return true, nil
	case task.IsSuccessful && task.IsCompleted:
		logger.Info("task is a success", "description", task.Type)
		scope.ProxmoxMachine.Status.TaskRef = nil
		return false, nil
	case task.IsFailed:
		// Failing tasks are actually red herrings. Some tasks fail, other
		// tasks can fail successfully (like qmstart).
		// We save the condition so the ReconcileVM statemachine keeps on working.
		conditionReason := conditions.GetReason(scope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)

		// qmstart can fail and yet actually start the VM. We can not handle qmstart properly.
		// In fact qmstart can find a machine already started, because proxmox's api is
		// eventually consistent here.
		// For all other jobs we do set the condition to failed.
		if task.Type != "qmstart" {
			logger.Info("task failed", "description", task.Type)
			// We notify the user that intervention is required. This should stop the state machine.
			conditionReason = infrav1.ProxmoxMachineVirtualMachineProvisionedTaskFailedReason
		}

		errorMessage := fmt.Sprintf("%s: %s", task.Type, task.ExitStatus)
		if task.ExitStatus == "OK" {
			// If you end up here, file a bug with go-proxmox.
			errorMessage = fmt.Sprintf("task %s failed but its exit status is OK; this should not happen", task.UPID)
		}

		conditions.Set(scope.ProxmoxMachine, metav1.Condition{
			Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  conditionReason,
			Message: errorMessage,
		})

		// Instead of directly requeuing the failed task, wait for the RetryAfter duration to pass
		// before resetting the taskRef from the ProxmoxMachine status.
		if scope.ProxmoxMachine.Status.RetryAfter.IsZero() {
			scope.ProxmoxMachine.Status.RetryAfter = &metav1.Time{Time: time.Now().Add(1 * time.Minute)}
		} else {
			scope.ProxmoxMachine.Status.TaskRef = nil
			scope.ProxmoxMachine.Status.RetryAfter = nil
		}
		return true, nil
	default:
		return false, NewRequeueError(fmt.Sprintf("unknown task state %q for %q", task.ExitStatus, scope.ProxmoxMachine.Name), infrav1.DefaultReconcilerRequeue)
	}
}
