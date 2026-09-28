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

	proxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

func TestDeleteVM_SuccessNotFound(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	proxmoxClient.EXPECT().CheckID(context.TODO(), int64(123)).Return(true, nil).Once()
	// no DeleteVM expectation: a destroy call fails the test

	require.NoError(t, DeleteVM(context.TODO(), machineScope))
	require.Empty(t, machineScope.ProxmoxMachine.Finalizers)
	require.Empty(t, machineScope.InfraCluster.ProxmoxCluster.GetNode(machineScope.Name(), false))
}

// A freed VMID is handed to the next clone within seconds. A stale record must
// never destroy the VM that now holds the ID.
func TestDeleteVM_SkipsVMWithAnotherName(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	vm.Name = "someone-else"
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	proxmoxClient.EXPECT().CheckID(context.TODO(), int64(123)).Return(false, nil).Once()
	proxmoxClient.EXPECT().FindVMResource(context.TODO(), uint64(123)).
		Return(&proxmox.ClusterResource{VMID: 123, Name: "someone-else", Node: "node1"}, nil).Once()
	proxmoxClient.EXPECT().GetVM(context.TODO(), "node1", int64(123)).Return(vm, nil).Once()
	// no DeleteVM expectation: a destroy call fails the test

	require.NoError(t, DeleteVM(context.TODO(), machineScope))
	require.Empty(t, machineScope.ProxmoxMachine.Finalizers)
	require.Empty(t, machineScope.InfraCluster.ProxmoxCluster.GetNode(machineScope.Name(), false))
}

func TestDeleteVM_DestroysOwnVM(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))

	proxmoxClient.EXPECT().CheckID(context.TODO(), int64(123)).Return(false, nil).Once()
	proxmoxClient.EXPECT().FindVMResource(context.TODO(), uint64(123)).
		Return(&proxmox.ClusterResource{VMID: 123, Name: "test", Node: "node1"}, nil).Once()
	proxmoxClient.EXPECT().GetVM(context.TODO(), "node1", int64(123)).Return(vm, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node1", int64(123)).Return(nil, nil).Once()

	require.NoError(t, DeleteVM(context.TODO(), machineScope))
}

// Just after a VMID is reused, /cluster/resources can still show the name of
// the previous holder. Only the VM config decides.
func TestDeleteVM_SkipsReusedVMIDWithStaleResourceName(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	vm.Name = "someone-else"
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	proxmoxClient.EXPECT().CheckID(context.TODO(), int64(123)).Return(false, nil).Once()
	proxmoxClient.EXPECT().FindVMResource(context.TODO(), uint64(123)).
		Return(&proxmox.ClusterResource{VMID: 123, Name: "test", Node: "node2"}, nil).Once()
	proxmoxClient.EXPECT().GetVM(context.TODO(), "node2", int64(123)).Return(vm, nil).Once()
	// no DeleteVM expectation: a destroy call fails the test

	require.NoError(t, DeleteVM(context.TODO(), machineScope))
	require.Empty(t, machineScope.ProxmoxMachine.Finalizers)
	require.Empty(t, machineScope.InfraCluster.ProxmoxCluster.GetNode(machineScope.Name(), false))
}

// A clone in progress has no name yet. It can be this machine's clone or the
// clone of another machine on a reused VMID, so wait until it has a name.
func TestDeleteVM_RequeuesWhileCloning(t *testing.T) {
	for _, name := range []string{"", "VM 123"} {
		t.Run(name, func(t *testing.T) {
			machineScope, proxmoxClient, _ := setupReconcilerTest(t)
			vm := newRunningVM()
			vm.Name = name
			machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
			machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
				Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
				Node:    "node1",
			}, false)

			proxmoxClient.EXPECT().CheckID(context.TODO(), int64(123)).Return(false, nil).Once()
			proxmoxClient.EXPECT().FindVMResource(context.TODO(), uint64(123)).
				Return(&proxmox.ClusterResource{VMID: 123, Name: name, Node: "node1"}, nil).Once()
			proxmoxClient.EXPECT().GetVM(context.TODO(), "node1", int64(123)).Return(vm, nil).Once()
			// no DeleteVM expectation: a destroy call fails the test

			require.ErrorIs(t, DeleteVM(context.TODO(), machineScope), ErrVMNotInitialized)
			require.NotEmpty(t, machineScope.ProxmoxMachine.Finalizers)
			require.NotEmpty(t, machineScope.InfraCluster.ProxmoxCluster.GetNode(machineScope.Name(), false))
		})
	}
}

// An unreadable cluster is not proof of anything. Requeue, keep the finalizer,
// and never destroy.
func TestDeleteVM_RequeuesWhenOwnershipIsUnknown(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(123))

	proxmoxClient.EXPECT().CheckID(context.TODO(), int64(123)).Return(false, errors.New("cannot get cluster")).Once()
	// no DeleteVM expectation: a destroy call fails the test

	require.Error(t, DeleteVM(context.TODO(), machineScope))
	require.NotEmpty(t, machineScope.ProxmoxMachine.Finalizers)
}

// Our own VM found on another node gets destroyed on that node.
func TestDeleteVM_DestroysOwnVMFoundClusterWide(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = new(int64(vm.VMID))
	machineScope.InfraCluster.ProxmoxCluster.AddNodeLocation(infrav1.NodeLocation{
		Machine: corev1.LocalObjectReference{Name: machineScope.Name()},
		Node:    "node1",
	}, false)

	proxmoxClient.EXPECT().CheckID(context.TODO(), int64(123)).Return(false, nil).Once()
	proxmoxClient.EXPECT().FindVMResource(context.TODO(), uint64(123)).
		Return(&proxmox.ClusterResource{VMID: 123, Name: "test", Node: "node2"}, nil).Once()
	proxmoxClient.EXPECT().GetVM(context.TODO(), "node2", int64(123)).Return(vm, nil).Once()
	proxmoxClient.EXPECT().DeleteVM(context.TODO(), "node2", int64(123)).Return(nil, nil).Once()

	require.NoError(t, DeleteVM(context.TODO(), machineScope))
}

// A machine that never got a VMID has nothing to destroy.
func TestDeleteVM_NoVMIDReleasesMachine(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.VirtualMachineID = nil
	// no client expectations: any Proxmox call fails the test

	require.NoError(t, DeleteVM(context.TODO(), machineScope))
	require.Empty(t, machineScope.ProxmoxMachine.Finalizers)
}
