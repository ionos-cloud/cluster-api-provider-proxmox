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

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
)

func TestReconcilePowerState_MissingIPAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	requeue, err := reconcilePowerState(context.TODO(), machineScope)
	require.True(t, requeue)
	require.NoError(t, err)
	require.Nil(t, machineScope.ProxmoxMachine.Status.TaskRef)
}

func TestReconcilePowerState_SetTaskRef(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}

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
	_, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newPausedVM()
	proxmoxClient.EXPECT().ResumeVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Stopped(t *testing.T) {
	ctx := context.TODO()
	_, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newStoppedVM()
	proxmoxClient.EXPECT().StartVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Hibernated(t *testing.T) {
	ctx := context.TODO()
	_, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newHibernatedVM()
	proxmoxClient.EXPECT().StartVM(ctx, vm).Return(newTask(), nil).Once()

	task, err := startVirtualMachine(ctx, proxmoxClient, vm)
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestStartVirtualMachine_Started(t *testing.T) {
	_, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()

	task, err := startVirtualMachine(context.TODO(), proxmoxClient, vm)
	require.NoError(t, err)
	require.Nil(t, task)
}
