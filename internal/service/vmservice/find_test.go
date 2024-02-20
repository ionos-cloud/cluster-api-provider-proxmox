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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
)

func TestFindVM_FindByNodeAndID(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))

	proxmoxClient.EXPECT().GetVM(ctx, "node1", int64(123)).Return(vm, nil).Once()

	_, err := FindVM(ctx, machineScope)
	require.NoError(t, err)
}

func TestFindVM_FindByNodeLocationsAndID(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1alpha1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.ProxmoxMachine.GetName()},
		Node:    "node3",
	}, false)

	proxmoxClient.EXPECT().GetVM(ctx, "node3", int64(123)).Return(vm, nil).Once()

	_, err := FindVM(ctx, machineScope)
	require.NoError(t, err)
}

func TestFindVM_NotFound(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.ProxmoxNode = ptr.To("node2")

	proxmoxClient.EXPECT().GetVM(ctx, "node2", int64(123)).Return(nil, errors.New("error")).Once()

	_, err := FindVM(ctx, machineScope)
	require.ErrorIs(t, err, ErrVMNotFound)
}

func TestFindVM_NotCreated(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	_, err := FindVM(context.TODO(), machineScope)
	require.ErrorIs(t, err, ErrVMNotCreated)
}

func TestFindVM_NotInitialized(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	vm.Name = "bar"
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.ProxmoxNode = ptr.To("node2")

	proxmoxClient.EXPECT().GetVM(ctx, "node2", int64(123)).Return(vm, nil).Once()

	_, err := FindVM(ctx, machineScope)
	require.ErrorIs(t, err, ErrVMNotInitialized)
}

func TestUpdateVMLocation_MissingName(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	vmr := newVMResource()
	vmr.Name = ""
	vm.VirtualMachineConfig.Name = ""
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))

	proxmoxClient.EXPECT().FindVMResource(ctx, uint64(123)).Return(vmr, nil).Once()
	proxmoxClient.EXPECT().GetVM(ctx, "node1", int64(123)).Return(vm, nil).Once()

	require.Error(t, updateVMLocation(ctx, machineScope))
}

func TestUpdateVMLocation_NameMismatch(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	vmr := newVMResource()
	name := "foo"
	vmr.Name = name
	vm.VirtualMachineConfig.Name = name
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))

	proxmoxClient.EXPECT().FindVMResource(ctx, uint64(123)).Return(vmr, nil).Once()
	proxmoxClient.EXPECT().GetVM(ctx, "node1", int64(123)).Return(vm, nil).Once()

	require.Error(t, updateVMLocation(ctx, machineScope))
	require.True(t, machineScope.HasFailed(), "expected failureReason and failureMessage to be set")
}

func TestUpdateVMLocation_UpdateNode(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	vmr := newVMResource()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.ProxmoxNode = ptr.To("node3")
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1alpha1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node3",
	}, false)

	proxmoxClient.EXPECT().FindVMResource(ctx, uint64(123)).Return(vmr, nil).Once()
	proxmoxClient.EXPECT().GetVM(ctx, "node1", int64(123)).Return(vm, nil).Once()

	require.NoError(t, updateVMLocation(ctx, machineScope))
	require.Equal(t, vmr.Node, *machineScope.ProxmoxMachine.Status.ProxmoxNode)
	require.Equal(t, vmr.Node, machineScope.InfraCluster.ProxmoxCluster.GetNode(machineScope.Name(), false))
}

func TestUpdateVMLocation_WithTask(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	vm := newRunningVM()

	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.TaskRef = ptr.To("test-task-uupid")

	require.Error(t, updateVMLocation(context.TODO(), machineScope))
}

func TestUpdateVMLocation_WithoutTaskNameMismatch(t *testing.T) {
	ctx := context.TODO()
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	vmr := newVMResource()
	name := "foo"
	vmr.Name = name
	vm.VirtualMachineConfig.Name = name
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.TaskRef = nil

	proxmoxClient.EXPECT().FindVMResource(ctx, uint64(123)).Return(vmr, nil).Once()
	proxmoxClient.EXPECT().GetVM(ctx, "node1", int64(123)).Return(vm, nil).Once()

	require.Error(t, updateVMLocation(ctx, machineScope))
	require.True(t, machineScope.HasFailed(), "expected failureReason and failureMessage to be set")
}
