/*
Copyright 2023-2025 IONOS Cloud.

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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1alpha2 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
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
	case task.IsSuccessful:
		logger.Info("task is a success", "description", task.Type)
		scope.ProxmoxMachine.Status.TaskRef = nil
		return false, nil
	case task.IsFailed:
		logger.Info("task failed", "description", task.Type)

		// NOTE: When a task fails there is not simple way to understand which operation is failing (e.g. cloning or powering on)
		// so we are reporting failures using a dedicated reason until we find a better solution.
		var errorMessage string

		if task.ExitStatus != "OK" {
			errorMessage = task.ExitStatus
		} else {
			errorMessage = "task failed but its exit status is OK; this should not happen"
		}
		conditions.MarkFalse(scope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition, infrav1alpha2.TaskFailure, clusterv1.ConditionSeverityInfo, "%s", errorMessage)

		// Instead of directly requeuing the failed task, wait for the RetryAfter duration to pass
		// before resetting the taskRef from the ProxmoxMachine status.
		if scope.ProxmoxMachine.Status.RetryAfter.IsZero() {
			scope.ProxmoxMachine.Status.RetryAfter = metav1.Time{Time: time.Now().Add(1 * time.Minute)}
		} else {
			scope.ProxmoxMachine.Status.TaskRef = nil
			scope.ProxmoxMachine.Status.RetryAfter = metav1.Time{}
		}
		return true, nil
	default:
		return false, NewRequeueError(fmt.Sprintf("unknown task state %q for %q", task.ExitStatus, scope.ProxmoxMachine.Name), infrav1alpha2.DefaultReconcilerRequeue)
	}
}
