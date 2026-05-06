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
	"strings"
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

// task type identifiers reported by Proxmox.
const (
	taskTypeQMStart = "qmstart"
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

	taskRef := *machineScope.ProxmoxMachine.Status.TaskRef
	task, err := machineScope.InfraCluster.ProxmoxClient.GetTask(ctx, taskRef)
	if err != nil {
		if isTaskNotFoundError(err) {
			return nil, fmt.Errorf("%w: %w", ErrTaskNotFound, err)
		}
		return nil, fmt.Errorf("received unknown task %s: %w", taskRef, err)
	}

	return task, nil
}

func isTaskNotFoundError(err error) bool {
	// go-proxmox currently returns task lookup failures as plain error text instead of a
	// typed sentinel; keep this heuristic narrow and covered by regression samples.
	// TODO: replace string matching if go-proxmox exposes a typed task-not-found error.
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") ||
		strings.Contains(message, "does not exist") ||
		strings.Contains(message, "no such task") ||
		strings.Contains(message, "task expired")
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
		if task.Type != taskTypeQMStart {
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
