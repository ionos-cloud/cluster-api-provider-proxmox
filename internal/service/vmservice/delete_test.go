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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/taskservice"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

const (
	deleteTestVMID = int64(123)
	stopUPID       = "UPID:node1:0000A:0000B:0000C:qmstop:123:root@pam:"
	destroyUPID    = "UPID:node1:0000A:0000B:0000C:qmdestroy:123:root@pam:"
)

// setupDeleteTest returns a machine that records VMID 123 on node1.
func setupDeleteTest(t *testing.T) (*scope.MachineScope, *proxmoxtest.MockClient) {
	t.Helper()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(deleteTestVMID)
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)
	return machineScope, proxmoxClient
}

// expectOwnedVM registers the lookups that locate vm under this machine's name on node.
func expectOwnedVM(proxmoxClient *proxmoxtest.MockClient, node string, vm *proxmox.VirtualMachine) {
	proxmoxClient.EXPECT().CheckID(context.TODO(), deleteTestVMID).Return(false, nil).Once()
	proxmoxClient.EXPECT().FindVMResource(context.TODO(), uint64(deleteTestVMID)).
		Return(&proxmox.ClusterResource{VMID: uint64(deleteTestVMID), Type: "qemu", Name: vm.Name, Node: node}, nil).Once()
	proxmoxClient.EXPECT().GetVM(context.TODO(), node, deleteTestVMID).Return(vm, nil).Once()
}

func requireReleased(t *testing.T, machineScope *scope.MachineScope) {
	t.Helper()
	require.Empty(t, machineScope.ProxmoxMachine.Finalizers)
	require.Empty(t, machineScope.InfraCluster.ProxmoxCluster.GetNode(machineScope.Name(), false))
}

func requireHeld(t *testing.T, machineScope *scope.MachineScope) {
	t.Helper()
	require.NotEmpty(t, machineScope.ProxmoxMachine.Finalizers)
	require.NotEmpty(t, machineScope.InfraCluster.ProxmoxCluster.GetNode(machineScope.Name(), false))
}

func requireReason(t *testing.T, machineScope *scope.MachineScope, reason string) {
	t.Helper()
	require.Equal(t, reason, conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
}

// A machine that never got a VMID, and has no VM carrying its name, has nothing to destroy.
func TestDeleteVM_NoVMIDReleasesMachine(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = nil
	// the setup answers the name lookup with "no such VM"; any other Proxmox call fails the test

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.Empty(t, machineScope.ProxmoxMachine.Finalizers)
	requireReason(t, machineScope, infrav1.ProxmoxMachineVirtualMachineProvisionedDeletingReason)
}

// No VMID is recorded, but the clone ran and its status update was lost: the VM carrying the
// machine's name is found and deleted rather than left behind.
func TestDeleteVM_NoVMIDFindsVMByName(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = nil
	proxmoxClient.ExpectedCalls = nil
	proxmoxClient.EXPECT().FindVMByName(context.TODO(), "test").
		Return(&proxmox.ClusterResource{VMID: uint64(deleteTestVMID), Node: "node1", Type: "qemu", Name: "test"}, nil).Once()
	vm := newStoppedVM()
	expectOwnedVM(proxmoxClient, "node1", vm)
	proxmoxClient.EXPECT().HasActiveTask(context.TODO(), vm).Return(false, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node1", deleteTestVMID).Return(&proxmox.Task{UPID: destroyUPID}, nil).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, deleteTestVMID, machineScope.GetVirtualMachineID())
	require.Equal(t, destroyUPID, *machineScope.ProxmoxMachine.Status.TaskRef)
	require.NotEmpty(t, machineScope.ProxmoxMachine.Finalizers)
}

// A free VMID means the VM is gone: release without calling destroy.
func TestDeleteVM_FreeVMIDReleasesMachine(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	proxmoxClient.EXPECT().CheckID(context.TODO(), deleteTestVMID).Return(true, nil).Once()
	// no DeleteVM expectation: a destroy call fails the test

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	requireReleased(t, machineScope)

	// The freed ID stays out of allocation for a while.
	reservedVMIDs.mu.Lock()
	defer reservedVMIDs.mu.Unlock()
	require.True(t, reservedVMIDs.held(deleteTestVMID))
}

// A freed VMID is handed to the next clone within seconds. A stale record must never
// destroy the VM that now holds the ID.
func TestDeleteVM_SkipsVMWithAnotherName(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newRunningVM()
	vm.Name = "someone-else"
	expectOwnedVM(proxmoxClient, "node1", vm)
	// no StopVM or DeleteVM expectation: either call fails the test

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	requireReleased(t, machineScope)
}

// Just after a VMID is reused, /cluster/resources can still show the name of the previous
// holder. Only the VM config decides.
func TestDeleteVM_SkipsReusedVMIDWithStaleResourceName(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newRunningVM()
	vm.Name = "someone-else"
	proxmoxClient.EXPECT().CheckID(context.TODO(), deleteTestVMID).Return(false, nil).Once()
	proxmoxClient.EXPECT().FindVMResource(context.TODO(), uint64(deleteTestVMID)).
		Return(&proxmox.ClusterResource{VMID: uint64(deleteTestVMID), Type: "qemu", Name: "test", Node: "node2"}, nil).Once()
	proxmoxClient.EXPECT().GetVM(context.TODO(), "node2", deleteTestVMID).Return(vm, nil).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	requireReleased(t, machineScope)
}

// Containers share the VMID space. A machine is never a container, even when the names match.
func TestDeleteVM_SkipsContainerOnReusedVMID(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	proxmoxClient.EXPECT().CheckID(context.TODO(), deleteTestVMID).Return(false, nil).Once()
	proxmoxClient.EXPECT().FindVMResource(context.TODO(), uint64(deleteTestVMID)).
		Return(&proxmox.ClusterResource{VMID: uint64(deleteTestVMID), Type: "lxc", Name: "test", Node: "node1"}, nil).Once()
	// no GetVM or DeleteVM expectation: either call fails the test

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	requireReleased(t, machineScope)
}

// A clone in progress has no name yet. It can be this machine's clone or the clone of another
// machine on a reused VMID, so wait until it has a name.
func TestDeleteVM_RequeuesWhileCloning(t *testing.T) {
	for _, name := range []string{"", "VM 123"} {
		t.Run(name, func(t *testing.T) {
			machineScope, proxmoxClient := setupDeleteTest(t)
			vm := newRunningVM()
			vm.Name = name
			expectOwnedVM(proxmoxClient, "node1", vm)

			requeue, err := DeleteVM(context.TODO(), machineScope)
			require.NoError(t, err)
			require.True(t, requeue)
			requireHeld(t, machineScope)
		})
	}
}

// An unreadable cluster is not proof of anything. Requeue, keep the finalizer, never destroy.
func TestDeleteVM_RequeuesWhenOwnershipIsUnknown(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	proxmoxClient.EXPECT().CheckID(context.TODO(), deleteTestVMID).Return(false, errors.New("cannot get cluster")).Once()

	_, err := DeleteVM(context.TODO(), machineScope)
	require.Error(t, err)
	requireHeld(t, machineScope)
}

// A running VM is stopped first; the stop task is recorded and nothing else is issued.
func TestDeleteVM_StopsRunningVMAndRecordsTask(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newRunningVM()
	expectOwnedVM(proxmoxClient, "node1", vm)
	proxmoxClient.EXPECT().HasActiveTask(context.TODO(), vm).Return(false, nil).Once()
	proxmoxClient.EXPECT().StopVM(context.TODO(), vm).Return(&proxmox.Task{UPID: stopUPID}, nil).Once()
	// no DeleteVM expectation: a destroy of a running VM fails the test

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, stopUPID, *machineScope.ProxmoxMachine.Status.TaskRef)
	requireHeld(t, machineScope)
	requireReason(t, machineScope, infrav1.ProxmoxMachineVirtualMachineProvisionedDeletingReason)
}

// A paused VM has status running too and needs the stop as well.
func TestDeleteVM_StopsPausedVM(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newPausedVM()
	vm.Name = "test"
	expectOwnedVM(proxmoxClient, "node1", vm)
	proxmoxClient.EXPECT().HasActiveTask(context.TODO(), vm).Return(false, nil).Once()
	proxmoxClient.EXPECT().StopVM(context.TODO(), vm).Return(&proxmox.Task{UPID: stopUPID}, nil).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, stopUPID, *machineScope.ProxmoxMachine.Status.TaskRef)
}

// A stopped VM is destroyed on the node that holds it; the destroy task is recorded and the
// finalizer stays until the VM is confirmed gone.
func TestDeleteVM_DestroysStoppedVMAndRecordsTask(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newStoppedVM()
	vm.VMID = proxmox.StringOrUint64(deleteTestVMID)
	vm.Node = "node2"
	expectOwnedVM(proxmoxClient, "node2", vm)
	proxmoxClient.EXPECT().HasActiveTask(context.TODO(), vm).Return(false, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node2", deleteTestVMID).Return(&proxmox.Task{UPID: destroyUPID}, nil).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, destroyUPID, *machineScope.ProxmoxMachine.Status.TaskRef)
	requireHeld(t, machineScope)
	requireReason(t, machineScope, infrav1.ProxmoxMachineVirtualMachineProvisionedDeletingReason)

	reservedVMIDs.mu.Lock()
	defer reservedVMIDs.mu.Unlock()
	require.True(t, reservedVMIDs.held(deleteTestVMID), "the destroyed ID is quarantined")
}

// A VM still locked by its clone cannot be destroyed yet.
func TestDeleteVM_WaitsForCloneLock(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newStoppedVM()
	vm.Lock = "clone"
	expectOwnedVM(proxmoxClient, "node1", vm)
	// no HasActiveTask, StopVM or DeleteVM expectation

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
	requireHeld(t, machineScope)
}

// A task on the VM that this pass does not know about (its status update was lost) is waited
// for instead of issuing a second stop or destroy against its lock.
func TestDeleteVM_WaitsForActiveTask(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newStoppedVM()
	expectOwnedVM(proxmoxClient, "node1", vm)
	proxmoxClient.EXPECT().HasActiveTask(context.TODO(), vm).Return(true, nil).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
	requireHeld(t, machineScope)
}

// While the recorded task runs nothing else is asked of Proxmox.
func TestDeleteVM_WaitsForRecordedTask(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new(destroyUPID)
	proxmoxClient.EXPECT().GetTask(context.TODO(), destroyUPID).
		Return(&proxmox.Task{UPID: destroyUPID, Type: "qmdestroy", Status: "running", IsRunning: true}, nil).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, destroyUPID, *machineScope.ProxmoxMachine.Status.TaskRef)
	requireHeld(t, machineScope)
}

// A finished destroy is followed by the check that the VM is really gone; only then does the
// finalizer come off.
func TestDeleteVM_ConfirmsVMGoneAfterDestroy(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new(destroyUPID)
	proxmoxClient.EXPECT().GetTask(context.TODO(), destroyUPID).
		Return(&proxmox.Task{UPID: destroyUPID, Type: "qmdestroy", Status: "stopped", ExitStatus: "OK", IsCompleted: true, IsSuccessful: true}, nil).Once()
	proxmoxClient.EXPECT().CheckID(context.TODO(), deleteTestVMID).Return(true, nil).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
	requireReleased(t, machineScope)
}

// A failed destroy ("VM 103 is running - destroy failed") sets DeletionFailed with the exit
// status, keeps the finalizer and waits RetryAfter before the next attempt.
func TestDeleteVM_FailedDestroySetsDeletionFailedAndRetries(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new(destroyUPID)
	failed := &proxmox.Task{UPID: destroyUPID, Type: "qmdestroy", Status: "stopped", ExitStatus: "VM 123 is running - destroy failed", IsCompleted: true, IsFailed: true}
	proxmoxClient.EXPECT().GetTask(context.TODO(), destroyUPID).Return(failed, nil).Times(2)

	// First pass after the failure: record it and start the wait.
	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireHeld(t, machineScope)
	requireReason(t, machineScope, infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason)
	cond := conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
	require.Equal(t, "qmdestroy: VM 123 is running - destroy failed", cond.Message)
	require.NotNil(t, machineScope.ProxmoxMachine.Status.RetryAfter)

	// While RetryAfter has not passed the pass asks for a timed requeue and does not touch
	// the VM; the failure stays visible.
	_, err = DeleteVM(context.TODO(), machineScope)
	var requeueErr *taskservice.RequeueError
	require.ErrorAs(t, err, &requeueErr)
	require.Positive(t, requeueErr.RequeueAfter())
	requireReason(t, machineScope, infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason)
	requireHeld(t, machineScope)
}

// Once RetryAfter has passed the failed task is dropped and the deletion starts over: the VM
// is located again and, still running, stopped.
func TestDeleteVM_RetriesAfterFailedDestroy(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new(destroyUPID)
	machineScope.ProxmoxMachine.Status.RetryAfter = &metav1.Time{Time: time.Now().Add(-time.Minute)}
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status: metav1.ConditionFalse,
		Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason,
	})
	failed := &proxmox.Task{UPID: destroyUPID, Type: "qmdestroy", Status: "stopped", ExitStatus: "VM 123 is running - destroy failed", IsCompleted: true, IsFailed: true}
	proxmoxClient.EXPECT().GetTask(context.TODO(), destroyUPID).Return(failed, nil).Once()

	// The expired wait clears the task; the task service asks for one more pass.
	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
	require.Nil(t, machineScope.ProxmoxMachine.Status.RetryAfter)

	vm := newRunningVM()
	expectOwnedVM(proxmoxClient, "node1", vm)
	proxmoxClient.EXPECT().HasActiveTask(context.TODO(), vm).Return(false, nil).Once()
	proxmoxClient.EXPECT().StopVM(context.TODO(), vm).Return(&proxmox.Task{UPID: stopUPID}, nil).Once()

	requeue, err = DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, stopUPID, *machineScope.ProxmoxMachine.Status.TaskRef)
	requireReason(t, machineScope, infrav1.ProxmoxMachineVirtualMachineProvisionedDeletingReason)
}

// A recorded task Proxmox no longer knows cannot be waited for: drop it and look at the VM.
func TestDeleteVM_UnknownTaskIsDropped(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new(destroyUPID)
	apiErr := &proxmox.APIError{StatusCode: 400, Status: "400 Parameter verification failed.", Errors: map[string]string{"upid": "no such task"}}
	proxmoxClient.EXPECT().GetTask(context.TODO(), destroyUPID).Return(nil, fmt.Errorf("cannot get task: %w", apiErr)).Once()
	proxmoxClient.EXPECT().CheckID(context.TODO(), deleteTestVMID).Return(true, nil).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
	requireReleased(t, machineScope)
}

// A task that could not be read (the API was unreachable) is kept and retried; nothing is
// decided about the VM meanwhile.
func TestDeleteVM_TaskReadErrorKeepsTask(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	machineScope.ProxmoxMachine.Status.TaskRef = new(destroyUPID)
	proxmoxClient.EXPECT().GetTask(context.TODO(), destroyUPID).
		Return(nil, &proxmox.APIError{StatusCode: 595, Status: "595 Connection refused"}).Once()

	_, err := DeleteVM(context.TODO(), machineScope)
	require.Error(t, err)
	require.Equal(t, destroyUPID, *machineScope.ProxmoxMachine.Status.TaskRef)
	requireHeld(t, machineScope)
}

// A destroy call the API rejects surfaces as DeletionFailed with the message, and keeps the finalizer.
func TestDeleteVM_DestroyErrorKeepsFinalizer(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newStoppedVM()
	expectOwnedVM(proxmoxClient, "node1", vm)
	proxmoxClient.EXPECT().HasActiveTask(context.TODO(), vm).Return(false, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node1", deleteTestVMID).Return(nil, errors.New("cannot delete vm with id 123: 403 Forbidden")).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.Error(t, err)
	require.False(t, requeue)
	requireHeld(t, machineScope)
	requireReason(t, machineScope, infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason)
	require.Contains(t, conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition).Message, "403 Forbidden")
}

// The VM vanished between the lookup and the destroy: it is gone, release.
func TestDeleteVM_DestroyNotFoundReleasesMachine(t *testing.T) {
	machineScope, proxmoxClient := setupDeleteTest(t)
	vm := newStoppedVM()
	expectOwnedVM(proxmoxClient, "node1", vm)
	proxmoxClient.EXPECT().HasActiveTask(context.TODO(), vm).Return(false, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node1", deleteTestVMID).
		Return(nil, errors.New("cannot find vm with id 123: 500 Configuration file 'nodes/node1/qemu-server/123.conf' does not exist")).Once()

	requeue, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	requireReleased(t, machineScope)
}
