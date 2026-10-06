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
	"testing"

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
)

func TestDeleteVM_SuccessNotFound(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeDestroyVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeStopVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node1", int64(123)).Return(nil, errors.New("vm does not exist: some reason")).Once()

	inFlight, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, inFlight)
	require.Empty(t, machineScope.ProxmoxMachine.Finalizers)
	require.Empty(t, machineScope.InfraCluster.ProxmoxCluster.GetNode(machineScope.Name(), false))
}

// A destroy task started by an earlier reconcile must be adopted rather than
// triggering another qmdestroy (#96).
func TestDeleteVM_AdoptsInFlightDestroyTask(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	task := &proxmox.Task{UPID: "UPID:node1:0000A:0000B:0000C:qmdestroy:123:root@pam:"}
	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeDestroyVM).Return(task, nil).Once()

	inFlight, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, inFlight)
	require.Equal(t, string(task.UPID), *machineScope.ProxmoxMachine.Status.TaskRef)
	// No DeleteVM call, and the finalizer stays until the task completes.
	require.NotEmpty(t, machineScope.ProxmoxMachine.Finalizers)
}

// Likewise for a stop task: the VM is on its way down, so do not re-issue.
func TestDeleteVM_AdoptsInFlightStopTask(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	task := &proxmox.Task{UPID: "UPID:node1:0000A:0000B:0000C:qmstop:123:root@pam:"}
	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeDestroyVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeStopVM).Return(task, nil).Once()

	inFlight, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, inFlight)
	require.Equal(t, string(task.UPID), *machineScope.ProxmoxMachine.Status.TaskRef)
}

// An error from the task lookup must abort the pass rather than fall through to
// issuing a destroy blind.
func TestDeleteVM_TaskLookupErrorAborts(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	proxmoxClient.EXPECT().
		GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeDestroyVM).
		Return(nil, errors.New("boom")).Once()

	inFlight, err := DeleteVM(context.TODO(), machineScope)
	require.Error(t, err)
	require.False(t, inFlight)
	// Finalizer must survive so the VM is not orphaned.
	require.NotEmpty(t, machineScope.ProxmoxMachine.Finalizers)
}

// The happy path: no task in flight, destroy issued, UPID recorded.
func TestDeleteVM_RecordsDestroyTask(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	task := &proxmox.Task{UPID: "UPID:node1:0001:qmdestroy:123:root@pam:"}
	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeDestroyVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeStopVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node1", int64(123)).Return(task, nil).Once()

	inFlight, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, inFlight)
	require.Equal(t, string(task.UPID), *machineScope.ProxmoxMachine.Status.TaskRef)
}

// A delete failure that is not "already gone" must surface and keep the finalizer.
func TestDeleteVM_DeleteErrorKeepsFinalizer(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeDestroyVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeStopVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node1", int64(123)).Return(nil, errors.New("VM is locked")).Once()

	inFlight, err := DeleteVM(context.TODO(), machineScope)
	require.Error(t, err)
	require.False(t, inFlight)
	require.NotEmpty(t, machineScope.ProxmoxMachine.Finalizers)
}

// Proxmox can accept the delete without handing back a task; nothing to track,
// so the caller must not be told to requeue on a task that does not exist.
func TestDeleteVM_NoTaskReturned(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeDestroyVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().GetVMActiveTask(context.TODO(), "node1", int64(123), goproxmox.TaskTypeStopVM).Return(nil, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node1", int64(123)).Return(nil, nil).Once()

	inFlight, err := DeleteVM(context.TODO(), machineScope)
	require.NoError(t, err)
	require.False(t, inFlight)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
}
