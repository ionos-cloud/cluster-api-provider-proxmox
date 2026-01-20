/*
Copyright 2023-2024 IONOS Cloud.

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

	"github.com/stretchr/testify/require"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

func TestReconcilePowerState_SetTaskRef(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForVMPowerUpReason)
	machineScope.ProxmoxMachine.Status.IPAddresses = []infrav1.IPAddressesSpec{{
		NetName: infrav1.DefaultNetworkDevice,
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

func TestStartVirtualMachine_Paused(t *testing.T) {
	ctx := context.TODO()
	_, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForVMPowerUpReason)
	vm := newPausedVM()
	proxmoxClient.EXPECT().ResumeVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Stopped(t *testing.T) {
	ctx := context.TODO()
	_, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForVMPowerUpReason)
	vm := newStoppedVM()
	proxmoxClient.EXPECT().StartVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Hibernated(t *testing.T) {
	ctx := context.TODO()
	_, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForVMPowerUpReason)
	vm := newHibernatedVM()
	proxmoxClient.EXPECT().StartVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Started(t *testing.T) {
	_, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForVMPowerUpReason)
	vm := newRunningVM()

	task, err := startVirtualMachine(context.TODO(), proxmoxClient, vm)
	require.NoError(t, err)
	require.Nil(t, task)
}
