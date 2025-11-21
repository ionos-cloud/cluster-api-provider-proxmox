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

package vmservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

const ipTag = "ip_net0_10.10.10.10"

func TestReconcileIPAddresses_CreateDefaultClaim(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_CreateAdditionalClaim(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	defaultPool := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "InClusterIPPool", Name: "custom"}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{Name: ptr.To("net0"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{defaultPool}}},
			{Name: ptr.To("net1"), InterfaceConfig: infrav1.InterfaceConfig{}},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)

	createIPPools(t, kubeClient, machineScope)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", &defaultPool)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NotNil(t, machineScope.ProxmoxMachine.Status.IPAddresses["net0"])
	require.Equal(t, *(machineScope.ProxmoxMachine.Status.IPAddresses["net0"]), infrav1.IPAddresses{IPV4: []string{"10.10.10.10"}, IPV6: nil})
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

/*func TestReconcileIPAddresses_AddIPTag(t *testing.T) {
	machineScope, proxmoxClient, kubeClient := setupReconcilerTest(t)
	vm := newStoppedVM()
	task := newTask()
	machineScope.SetVirtualMachine(vm)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10")

	proxmoxClient.EXPECT().TagVM(context.Background(), vm, ipTag).Return(task, nil).Once()

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}*/

func TestReconcileIPAddresses_SetIPAddresses(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	defaultPool := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "InClusterIPPool", Name: "default"}
	extraPool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "GlobalInClusterIPPool", Name: "additional"}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{Name: ptr.To("net0"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{defaultPool}}},
			{Name: ptr.To("net1"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{extraPool0}}},
		},
	}
	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", &defaultPool)
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10", &extraPool0)
	createIPPools(t, kubeClient, machineScope)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)

	require.NoError(t, err)
	require.True(t, requeue)

	require.NotNil(t, machineScope.ProxmoxMachine.Status.IPAddresses["net0"])
	require.Equal(t, *(machineScope.ProxmoxMachine.Status.IPAddresses["net0"]), infrav1.IPAddresses{IPV4: []string{"10.10.10.10"}, IPV6: nil})
	require.NotNil(t, machineScope.ProxmoxMachine.Status.IPAddresses["net1"])
	require.Equal(t, *(machineScope.ProxmoxMachine.Status.IPAddresses["net1"]), infrav1.IPAddresses{IPV4: []string{"10.100.10.10"}, IPV6: nil})

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_MultipleDevices(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	ipv4pool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "GlobalInClusterIPPool", Name: "ipv4pool0"}
	ipv4pool1 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "GlobalInClusterIPPool", Name: "ipv4pool1"}
	ipv6pool := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "GlobalInClusterIPPool", Name: "ipv6pool"}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{Name: ptr.To(infrav1.DefaultNetworkDevice), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{ipv4pool0}}},
			{Name: ptr.To("net1"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{ipv4pool1}}},
			{Name: ptr.To("net2"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{ipv6pool}}},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)

	createIP4AddressResource(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", &ipv4pool0)
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10", &ipv4pool1)
	createIP6AddressResource(t, kubeClient, machineScope, "net2", "fe80::ffee", &ipv6pool)
	createIPPools(t, kubeClient, machineScope)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	//fmt.Println("reconcileIPAddresses", requeue, err)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Len(t, machineScope.ProxmoxMachine.Status.IPAddresses, 3)

	expected := map[string]*infrav1.IPAddresses{
		"net0": {IPV4: []string{"10.10.10.10"}},
		"net1": {IPV4: []string{"10.100.10.10"}},
		"net2": {IPV6: []string{"fe80::ffee"}},
	}

	require.Equal(t, expected, machineScope.ProxmoxMachine.Status.IPAddresses)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_IPV6(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	defaultPool := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "InClusterIPPool", Name: "default"}
	defaultPoolV6 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "InClusterIPPool", Name: "defaultV6"}
	extraPool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "GlobalInClusterIPPool", Name: "additional"}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{Name: ptr.To("net0"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{defaultPool, defaultPoolV6}}},
			{Name: ptr.To("net1"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{extraPool0}}},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", &defaultPool)
	createIP6AddressResource(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "fe80::1", &defaultPoolV6)
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10", &extraPool0)
	createIPPools(t, kubeClient, machineScope)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	require.NotNil(t, machineScope.ProxmoxMachine.Status.IPAddresses["net0"])
	require.Equal(t, *(machineScope.ProxmoxMachine.Status.IPAddresses["net0"]), infrav1.IPAddresses{IPV4: []string{"10.10.10.10"}, IPV6: []string{"fe80::1"}})
	require.NotNil(t, machineScope.ProxmoxMachine.Status.IPAddresses["net1"])
	require.Equal(t, *(machineScope.ProxmoxMachine.Status.IPAddresses["net1"]), infrav1.IPAddresses{IPV4: []string{"10.100.10.10"}, IPV6: nil})

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

func TestReconcileIPAddresses_MachineIPPoolRef(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{{Kind: "GlobalInClusterIPPool", Name: "custom-ips"}}}},
			{Name: ptr.To("net1"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{{Kind: "GlobalInClusterIPPool", Name: "custom-additional-ips"}}}},
		},
	}

	defaultPool := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "InClusterIPPool", Name: "default"}
	extraPool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"), Kind: "GlobalInClusterIPPool", Name: "additional"}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{Name: ptr.To("net0"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{defaultPool}}},
			{Name: ptr.To("net1"), InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{extraPool0}}},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)

	createIP4AddressResource(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", &defaultPool)
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10", &extraPool0)
	createIPPools(t, kubeClient, machineScope)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	require.NotNil(t, machineScope.ProxmoxMachine.Status.IPAddresses["net0"])
	require.Equal(t, *(machineScope.ProxmoxMachine.Status.IPAddresses["net0"]), infrav1.IPAddresses{IPV4: []string{"10.10.10.10"}, IPV6: nil})
	require.NotNil(t, machineScope.ProxmoxMachine.Status.IPAddresses["net1"])
	require.Equal(t, *(machineScope.ProxmoxMachine.Status.IPAddresses["net1"]), infrav1.IPAddresses{IPV4: []string{"10.100.10.10"}, IPV6: nil})

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}
