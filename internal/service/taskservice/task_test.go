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

package taskservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func setupTaskTest(t *testing.T) (*scope.MachineScope, *proxmoxtest.MockClient) {
	t.Helper()

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
		},
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
		},
	}

	infraCluster := &infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: infrav1.ProxmoxClusterSpec{
			IPv4Config: &infrav1.IPConfigSpec{
				Addresses: []string{"192.0.2.10-192.0.2.20"},
				Prefix:    24,
				Gateway:   "192.0.2.1",
			},
		},
		Status: infrav1.ProxmoxClusterStatus{
			NodeLocations: &infrav1.NodeLocations{},
		},
	}

	infraMachine := &infrav1.ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: infrav1.ProxmoxMachineSpec{
			Network: new(infrav1.NetworkSpec{}),
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1.AddToScheme(scheme))
	require.NoError(t, infrav1.AddToScheme(scheme))

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, machine, infraCluster, infraMachine).
		WithStatusSubresource(&infrav1.ProxmoxCluster{}, &infrav1.ProxmoxMachine{}).
		Build()

	logger := logr.Discard()
	mockClient := proxmoxtest.NewMockClient(t)
	ipamHelper := ipam.NewHelper(kubeClient, infraCluster)

	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Client:         kubeClient,
		Logger:         &logger,
		Cluster:        cluster,
		ProxmoxCluster: infraCluster,
		ProxmoxClient:  mockClient,
		IPAMHelper:     ipamHelper,
	})
	require.NoError(t, err)

	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Client:         kubeClient,
		Logger:         &logger,
		Cluster:        cluster,
		Machine:        machine,
		InfraCluster:   clusterScope,
		ProxmoxMachine: infraMachine,
		IPAMHelper:     ipamHelper,
	})
	require.NoError(t, err)

	return machineScope, mockClient
}

// Do not return task if there is no task.
func TestGetTask_NoTaskRef(t *testing.T) {
	machineScope, _ := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = nil

	task, err := GetTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.Nil(t, task)
}

// Test missing task erroring.
func TestGetTask_ErrorNotFound(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(nil, errors.New("not found")).Once()

	task, err := GetTask(context.Background(), machineScope)
	require.ErrorIs(t, err, ErrTaskNotFound)
	require.Nil(t, task)
}

// Test successful task returning.
func TestGetTask_Success(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	expected := &proxmox.Task{UPID: "UPID:node1:001", IsCompleted: true, IsSuccessful: true}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(expected, nil).Once()

	task, err := GetTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, expected, task)
}

// Test ReconcileInFlightTask on empty task.
func TestReconcileInFlightTask_NoTaskRef(t *testing.T) {
	machineScope, _ := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = nil

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
}

// Test ReconcileInFlightTask on empty task but existing TaskRef.
func TestReconcileInFlightTask_NilTaskReturned(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(nil, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
}

// Test ReconcileInflightTask on running task switch case.
func TestReconcileInFlightTask_TaskRunning(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	task := &proxmox.Task{UPID: "UPID:node1:001", IsRunning: true, Status: "running", Type: "qmclone"}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(task, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
}

// Test ReconcileInflightTask on successful task switch case.
func TestReconcileInFlightTask_TaskSuccessful(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	task := &proxmox.Task{UPID: "UPID:node1:001", IsCompleted: true, IsSuccessful: true, Status: "stopped", ExitStatus: "OK", Type: "qmclone"}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(task, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
}

// Test ReconcileInflightTask on task failure switch case if not qmstart.
func TestReconcileInFlightTask_CloneTaskFailed(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	// Set an initial condition to verify it gets overwritten.
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status: metav1.ConditionFalse,
		Reason: "SomeOtherReason",
	})

	task := &proxmox.Task{UPID: "UPID:node1:001", IsFailed: true, IsCompleted: true, Status: "stopped", ExitStatus: "ERROR: clone failed", Type: "qmclone"}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(task, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	// First failure: RetryAfter should be set, TaskRef should still be present.
	require.NotNil(t, machineScope.ProxmoxMachine.Status.RetryAfter)
	require.NotNil(t, machineScope.ProxmoxMachine.Status.TaskRef)

	// Condition should be set to TaskFailed for non-qmstart tasks.
	cond := conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
	require.NotNil(t, cond)
	require.Equal(t, infrav1.ProxmoxMachineVirtualMachineProvisionedTaskFailedReason, cond.Reason)
	require.Contains(t, cond.Message, "ERROR: clone failed")
}

// Test ReconcileInflightTask on task failure switch case if qmstart (special case failure).
func TestReconcileInFlightTask_TaskFailed_QMStart(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	// Set a pre-existing condition that should be preserved for qmstart.
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status: metav1.ConditionFalse,
		Reason: "WaitingForVMPowerUp",
	})

	task := &proxmox.Task{UPID: "UPID:node1:001", IsFailed: true, IsCompleted: true, Status: "stopped", ExitStatus: "ERROR: VM already running", Type: "qmstart"}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(task, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	// Condition reason should be preserved (not set to TaskFailed) for qmstart.
	cond := conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
	require.NotNil(t, cond)
	require.Equal(t, "WaitingForVMPowerUp", cond.Reason)
}

// Test ReconcileInflightTask on task failure switch case clears timed out task.
func TestReconcileInFlightTask_TaskFailed_SecondPass_ClearsTaskRef(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	// Simulate second reconciliation pass: RetryAfter is already set and expired.
	machineScope.ProxmoxMachine.Status.RetryAfter = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}

	task := &proxmox.Task{UPID: "UPID:node1:001", IsFailed: true, IsCompleted: true, Status: "stopped", ExitStatus: "ERROR: clone failed", Type: "qmclone"}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(task, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	// Second failure pass: both TaskRef and RetryAfter should be cleared.
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
	require.Nil(t, machineScope.ProxmoxMachine.Status.RetryAfter)
}

// Test ReconcileInflightTask on invalid task state in go-proxmox.
func TestReconcileInFlightTask_TaskFailed_ExitStatusOK(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	task := &proxmox.Task{UPID: "UPID:node1:001", IsFailed: true, IsCompleted: true, Status: "stopped", ExitStatus: "OK", Type: "qmclone"}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(task, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	cond := conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
	require.NotNil(t, cond)
	require.Contains(t, cond.Message, "failed but its exit status is OK")
}

// Test ReconcileInflightTask failed task time-out.
func TestReconcileInFlightTask_RetryAfterNotExpired(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")
	machineScope.ProxmoxMachine.Status.RetryAfter = &metav1.Time{Time: time.Now().Add(5 * time.Minute)}

	task := &proxmox.Task{UPID: "UPID:node1:001", IsFailed: true, IsCompleted: true, Status: "stopped", ExitStatus: "ERROR", Type: "qmclone"}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(task, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.False(t, requeue)

	var requeueErr *RequeueError
	require.ErrorAs(t, err, &requeueErr)
	require.Positive(t, requeueErr.RequeueAfter())
}

// Test ReconcileInflightTask unknown state switch case.
func TestReconcileInFlightTask_UnknownState(t *testing.T) {
	machineScope, mockClient := setupTaskTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new("UPID:node1:001")

	// Task with no state flags set falls through to default case.
	task := &proxmox.Task{UPID: "UPID:node1:001", ExitStatus: "weird-state", Type: "qmclone"}
	mockClient.EXPECT().GetTask(context.Background(), "UPID:node1:001").Return(task, nil).Once()

	requeue, err := ReconcileInFlightTask(context.Background(), machineScope)
	require.False(t, requeue)

	var requeueErr *RequeueError
	require.ErrorAs(t, err, &requeueErr)
}
