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
	"testing"

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
)

func TestReconcilePowerState_SetTaskRef(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason)
	machineScope.ProxmoxMachine.Status.IPAddresses = []infrav1.IPAddressesSpec{{
		NetName: string(infrav1.DefaultNetworkDevice),
		IPv4:    []string{"10.10.10.10"},
	}, {
		NetName: "default",
		IPv4:    []string{"10.10.10.10"},
	}}

	vm := newStoppedVM()
	task := newTask()
	machineScope.SetVirtualMachine(vm)
	proxmoxClient.EXPECT().StartVM(ctx, vm).Return(task, nil).Once()

	requeue, err := reconcilePowerState(ctx, machineScope)
	require.True(t, requeue)
	require.NoError(t, err)
	require.NotEmpty(t, *machineScope.ProxmoxMachine.Status.TaskRef)
}

// Regression test for the duplicate qmstart: a reconcile that runs before the
// previous pass's Status.TaskRef write has propagated through the cache must
// not issue a second start. StartVM is deliberately not expected here - the
// mock fails the test if it is called (#727).
func TestReconcilePowerState_AdoptsInFlightStartTask(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason)

	vm := newStoppedVM()
	vm.VMID = 123
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(123))
	machineScope.SetVirtualMachine(vm)
	// Cache lag: the ref from the pass that already started the VM is not visible.
	machineScope.ProxmoxMachine.Status.TaskRef = nil

	task := &proxmox.Task{UPID: "UPID:node1:0000A:0000B:0000C:qmstart:123:root@pam:"}
	proxmoxClient.EXPECT().
		GetVMActiveTask(ctx, machineScope.LocateProxmoxNode(), int64(123), goproxmox.TaskTypeStartVM).
		Return(task, nil).Once()

	requeue, err := reconcilePowerState(ctx, machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, string(task.UPID), *machineScope.ProxmoxMachine.Status.TaskRef)
}

// With no start in flight the VM must still be started as before.
func TestReconcilePowerState_StartsWhenNoTaskInFlight(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason)

	vm := newStoppedVM()
	vm.VMID = 123
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(123))
	machineScope.SetVirtualMachine(vm)

	started := newTask()
	proxmoxClient.EXPECT().
		GetVMActiveTask(ctx, machineScope.LocateProxmoxNode(), int64(123), goproxmox.TaskTypeStartVM).
		Return(nil, nil).Once()
	proxmoxClient.EXPECT().StartVM(ctx, vm).Return(started, nil).Once()

	requeue, err := reconcilePowerState(ctx, machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, string(started.UPID), *machineScope.ProxmoxMachine.Status.TaskRef)
}

// A running VM is not a duplicate-start candidate, so Proxmox must not be
// queried at all - the state machine just advances.
func TestReconcilePowerState_RunningVMSkipsTaskLookup(t *testing.T) {
	ctx := context.TODO()
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason)

	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(123))
	machineScope.SetVirtualMachine(vm)

	requeue, err := reconcilePowerState(ctx, machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
}

func TestStartVirtualMachine_Paused(t *testing.T) {
	ctx := context.TODO()
	_, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason)
	vm := newPausedVM()
	proxmoxClient.EXPECT().ResumeVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Stopped(t *testing.T) {
	ctx := context.TODO()
	_, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason)
	vm := newStoppedVM()
	proxmoxClient.EXPECT().StartVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Hibernated(t *testing.T) {
	ctx := context.TODO()
	_, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason)
	vm := newHibernatedVM()
	proxmoxClient.EXPECT().StartVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Started(t *testing.T) {
	_, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason)
	vm := newRunningVM()

	task, err := startVirtualMachine(context.TODO(), proxmoxClient, vm)
	require.NoError(t, err)
	require.Nil(t, task)
}
