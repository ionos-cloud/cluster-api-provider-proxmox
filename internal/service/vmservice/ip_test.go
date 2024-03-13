/*
Copyright 2023 IONOS Cloud.

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
	corev1 "k8s.io/api/core/v1"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
)

const ipTag = "ip_net0_10.10.10.10"

func TestReconcileIPAddresses_CreateDefaultClaim(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_CreateAdditionalClaim(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{Name: "net1", IPv4PoolRef: &corev1.TypedLocalObjectReference{Kind: "InClusterIPPool", Name: "custom"}},
		},
	}
	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createIPPools(t, kubeClient, machineScope)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_AddIPTag(t *testing.T) {
	machineScope, proxmoxClient, kubeClient := setupReconcilerTest(t)
	vm := newStoppedVM()
	task := newTask()
	machineScope.SetVirtualMachine(vm)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")

	proxmoxClient.EXPECT().TagVM(context.Background(), vm, ipTag).Return(task, nil).Once()

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_SetIPAddresses(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{Name: "net1", IPv4PoolRef: &corev1.TypedLocalObjectReference{Kind: "GlobalInClusterIPPool", Name: "custom"}},
		},
	}
	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10")
	createIPPools(t, kubeClient, machineScope)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_MultipleDevices(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{Name: "net1", IPv4PoolRef: &corev1.TypedLocalObjectReference{Kind: "GlobalInClusterIPPool", Name: "ipv4pool"}},
			{Name: "net2", IPv6PoolRef: &corev1.TypedLocalObjectReference{Kind: "GlobalInClusterIPPool", Name: "ipv6pool"}},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)

	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10")
	createIP6AddressResource(t, kubeClient, machineScope, "net2", "fe80::ffee")
	createIPPools(t, kubeClient, machineScope)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Len(t, machineScope.ProxmoxMachine.Status.IPAddresses, 3)

	expected := map[string]infrav1alpha1.IPAddress{
		"net0": {IPV4: "10.10.10.10"},
		"net1": {IPV4: "10.100.10.10"},
		"net2": {IPV6: "fe80::ffee"},
	}

	require.Equal(t, expected, machineScope.ProxmoxMachine.Status.IPAddresses)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_IPV6(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config = &ipamicv1.InClusterIPPoolSpec{
		Addresses: []string{"fe80::/64"},
		Prefix:    64,
		Gateway:   "fe80::1",
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{Name: "net1", IPv4PoolRef: &corev1.TypedLocalObjectReference{Kind: "GlobalInClusterIPPool", Name: "custom"}},
		},
	}
	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createIP6AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "fe80::1")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10")
	createIPPools(t, kubeClient, machineScope)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}
