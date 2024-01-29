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

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

const (
	biosUUID = "uuid=41ec1197-580f-460b-b41b-1dfefabe6e32"
)

func TestReconcileBootstrapData_MissingIPAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}

func TestReconcileBootstrapData_MissingMACAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.SetVirtualMachine(newStoppedVM())
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
}

func TestReconcileBootstrapData_NoNetworkConfig_UpdateStatus(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createBootstrapSecret(t, kubeClient, machineScope)

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestReconcileBootstrapData_UpdateStatus(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{
				NetworkDevice: infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")},
				Name:          "net1",
				DNSServers:    []string{"1.2.3.4"},
			},
		},
	}
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}, "net1": {IPV4: "10.100.10.10"}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10")
	createBootstrapSecret(t, kubeClient, machineScope)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestGetBootstrapData_MissingSecretName(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	data, err := getBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.Nil(t, data)
}

func TestGetNetworkConfigDataForDevice_MissingIPAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	machineScope.SetVirtualMachine(vm)

	cfg, err := getNetworkConfigDataForDevice(context.Background(), machineScope, "net0")
	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestGetNetworkConfigDataForDevice_MissingMACAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.SetVirtualMachine(newStoppedVM())

	cfg, err := getNetworkConfigDataForDevice(context.Background(), machineScope, "net2")
	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestGetRoutingDataMock(t *testing.T) {
	// The underlying copy code can not fail. This test only exists for coverage
	routes := *getRoutingData([]infrav1alpha1.RouteSpec{
		{To: "default", Via: "192.168.178.1"},
		{To: "172.24.16.0/24", Via: "192.168.178.1", Table: 100},
	})

	require.Equal(t, "default", routes[0].To)
	require.NoError(t, nil)
}

func TestGetRoutingpolicyDataMock(t *testing.T) {
	// The underlying copy code can not fail. This test only exists for coverage
	rules := *getRoutingPolicyData([]infrav1alpha1.RoutingPolicySpec{
		{To: "10.10.10.0/24", Table: 100},
		{From: "172.24.16.0/24", Table: 100},
	})

	require.Equal(t, "10.10.10.0/24", rules[0].To)
	require.NoError(t, nil)
}

func TestGetVirtualNetworkDevices_VrfDevice_MissingInterface(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.SetVirtualMachine(newStoppedVM())

	networkSpec := infrav1alpha1.NetworkSpec{
		VirtualNetworkDevices: infrav1alpha1.VirtualNetworkDevices{
			VRFs: []infrav1alpha1.VrfDevice{{
				Name:       "vrf-blue",
				Table:      500,
				Interfaces: []string{"net1"},
			}},
		},
	}
	networkConfigData := []cloudinit.NetworkConfigData{{}}

	cfg, err := getVirtualNetworkDevices(context.Background(), machineScope, networkSpec, networkConfigData)
	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestReconcileBootstrapData_DualStack(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config = &v1alpha2.InClusterIPPoolSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10", IPV6: "2001:db8::2"}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createIP6AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "2001:db8::2")

	createBootstrapSecret(t, kubeClient, machineScope)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestReconcileBootstrapData_DualStack_AdditionalDevices(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config = &v1alpha2.InClusterIPPoolSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{
				NetworkDevice: infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")},
				Name:          "net1",
				DNSServers:    []string{"1.2.3.4"},
				IPv6PoolRef: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
					Kind:     "GlobalInClusterIPPool",
					Name:     "sample",
				},
				IPv4PoolRef: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
					Kind:     "InClusterIPPool",
					Name:     "sample",
				},
			},
		},
	}

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10", IPV6: "2001:db8::2"}, "net1": {IPV4: "10.0.0.10", IPV6: "2001:db8::9"}}
	createIPPools(t, kubeClient, machineScope)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createIP6AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "2001:db8::2")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.0.0.10")
	createIP6AddressResource(t, kubeClient, machineScope, "net1", "2001:db8::9")
	createBootstrapSecret(t, kubeClient, machineScope)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestReconcileBootstrapData_VirtualDevices_VRF(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		VirtualNetworkDevices: infrav1alpha1.VirtualNetworkDevices{
			VRFs: []infrav1alpha1.VrfDevice{{
				Interfaces: []string{"net1"},
				Name:       "vrf-blue",
				Table:      500,
			}},
		},
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{
				NetworkDevice: infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")},
				Name:          "net1",
				DNSServers:    []string{"1.2.3.4"},
				IPv4PoolRef: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
					Kind:     "InClusterIPPool",
					Name:     "sample",
				},
			},
		},
	}
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}, "net1": {IPV4: "10.100.10.10"}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10")

	createBootstrapSecret(t, kubeClient, machineScope)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestVMHasMacAddress(t *testing.T) {
	machineScope := &scope.MachineScope{VirtualMachine: newRunningVM()}
	require.False(t, vmHasMacAddresses(machineScope))
	machineScope.VirtualMachine = newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr1")
	require.True(t, vmHasMacAddresses(machineScope))
	machineScope.VirtualMachine = newVMWithNets("bridge=vmbr1")
	require.False(t, vmHasMacAddresses(machineScope))
}

func TestReconcileBootstrapDataMissingSecret(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)

	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha1.DefaultNetworkDevice, "10.10.10.10")

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, conditions.IsFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, conditions.GetReason(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition) == infrav1alpha1.CloningFailedReason)
}

func TestReconcileBootstrapDataMissingNetworkConfig(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)

	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	createBootstrapSecret(t, kubeClient, machineScope)

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, conditions.IsFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition))
	require.True(t, conditions.GetReason(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition) == infrav1alpha1.WaitingForStaticIPAllocationReason)
}

func TestDefaultISOInjector(t *testing.T) {
	injector := defaultISOInjector(newRunningVM(), []byte("data"), cloudinit.NewMetadata(biosUUID, "test"), cloudinit.NewNetworkConfig(nil))

	require.NotEmpty(t, injector)
	require.Equal(t, []byte("data"), injector.(*inject.ISOInjector).BootstrapData)
}
