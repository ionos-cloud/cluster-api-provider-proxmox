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
	"errors"
	"testing"

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1alpha2 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
)

const (
	biosUUID = "uuid=41ec1197-580f-460b-b41b-1dfefabe6e32"
)

var defaultNic = infrav1alpha2.NetworkDevice{
	Bridge: "vmbr0",
	Model:  ptr.To("virtio"),
	Name:   "net0",
}

func TestReconcileBootstrapData_MissingIPAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition)
}

func TestReconcileBootstrapData_MissingMACAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.SetVirtualMachine(newStoppedVM())
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}}

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
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
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestReconcileBootstrapData_UpdateStatus(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha2.NetworkSpec{
		NetworkDevices: []infrav1alpha2.NetworkDevice{
			defaultNic,
			{
				Bridge: "vmbr1",
				Model:  ptr.To("virtio"),
				Name:   "net1",
				InterfaceConfig: infrav1alpha2.InterfaceConfig{
					DNSServers: []string{"1.2.3.4"},
				},
			},
		},
	}
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}, "net1": {IPV4: []string{"10.100.10.10"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10")
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestReconcileBootstrapData_BadInjector(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}, "net1": {IPV4: []string{"10.100.10.10"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{Error: errors.New("bad FakeISOInjector")}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to inject bootstrap data: bad FakeISOInjector")
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.Nil(t, machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestGetBootstrapData_MissingSecretName(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	data, _, err := getBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.Equal(t, err.Error(), "machine has no bootstrap data")
	require.Nil(t, data)
}

func TestGetBootstrapData_MissingSecretNotName(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	machineScope.Machine.Spec.Bootstrap.DataSecretName = ptr.To("foo")
	data, _, err := getBootstrapData(context.Background(), machineScope)

	require.Error(t, err)
	require.Equal(t, err.Error(), "failed to retrieve bootstrap data secret: secrets \"foo\" not found")
	require.Nil(t, data)
}

func TestGetBootstrapData_MissingSecretValue(t *testing.T) {
	machineScope, _, client := setupReconcilerTest(t)

	machineScope.Machine.Spec.Bootstrap.DataSecretName = ptr.To(machineScope.Name())
	// missing format
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machineScope.Name(),
			Namespace: machineScope.Namespace(),
		},
		Data: map[string][]byte{
			"notvalue": []byte("notdata"),
		},
	}
	require.NoError(t, client.Create(context.Background(), secret))

	// missing value
	data, _, err := getBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.Equal(t, "error retrieving bootstrap data: secret `value` key is missing", err.Error())
	require.Nil(t, data)

	secret.Data["value"] = []byte("notdata")
	require.NoError(t, client.Update(context.Background(), secret))

	data, _, err = getBootstrapData(context.Background(), machineScope)
	require.Equal(t, []byte("notdata"), data)
	require.Nil(t, err)
}

// TODO: Do we need to error on not having a pool?
/*
func TestGetNetworkConfigDataForDevice_MissingIPAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	machineScope.SetVirtualMachine(vm)

	cfg, err := getNetworkConfigDataForDevice(context.Background(), machineScope, "net0", nil)
	require.Error(t, err)
	require.Nil(t, cfg)
}
*/

func TestGetNetworkConfigDataForDevice_MissingMACAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.SetVirtualMachine(newStoppedVM())

	cfg, err := getNetworkConfigDataForDevice(context.Background(), machineScope, "net2", nil)
	require.Error(t, err)
	require.Nil(t, cfg)
}

// TODO: getCommonInterfaceConfig no longer sets IPs
/*
func TestGetCommonInterfaceConfig_MissingIPPool(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha2.NetworkSpec{
		NetworkDevices: []infrav1alpha2.NetworkDevice{
			defaultNic,
			{
				Bridge: "vmbr1",
				Model:  ptr.To("virtio"),
				Name:   "net1",
				InterfaceConfig: infrav1alpha2.InterfaceConfig{
					IPPoolRef: []corev1.TypedLocalObjectReference{{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "GlobalInClusterIPPool",
						Name:     "net1-inet",
					}},
				},
			},
		},
	}

	cfg := &types.NetworkConfigData{Name: "net1"}
	err := getCommonInterfaceConfig(context.Background(), machineScope, cfg, machineScope.ProxmoxMachine.Spec.Network.NetworkDevices[0].InterfaceConfig)
	require.Error(t, err)
}
*/

func TestGetCommonInterfaceConfig_NoIPAddresses(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha2.NetworkSpec{
		NetworkDevices: []infrav1alpha2.NetworkDevice{
			{
				Bridge: "vmbr1",
				Model:  ptr.To("virtio"),
				Name:   "net1",
			},
		},
	}

	cfg := &types.NetworkConfigData{Name: "net1"}
	err := getCommonInterfaceConfig(context.Background(), machineScope, cfg, machineScope.ProxmoxMachine.Spec.Network.NetworkDevices[0].InterfaceConfig)
	require.NoError(t, err)
}

func TestGetCommonInterfaceConfig(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	var MTU uint16 = 9000
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha2.NetworkSpec{
		NetworkDevices: []infrav1alpha2.NetworkDevice{
			{
				Bridge: "vmbr1",
				Model:  ptr.To("virtio"),
				Name:   "net1",
				InterfaceConfig: infrav1alpha2.InterfaceConfig{
					DNSServers: []string{"1.2.3.4"},
					IPPoolRef: []corev1.TypedLocalObjectReference{{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "GlobalInClusterIPPool",
						Name:     "net1-inet6",
					}, {
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "GlobalInClusterIPPool",
						Name:     "net1-inet",
					}},
					LinkMTU: &MTU,
					Routing: infrav1alpha2.Routing{
						Routes: []infrav1alpha2.RouteSpec{
							{To: "default", Via: "192.168.178.1"},
							{To: "172.24.16.0/24", Via: "192.168.178.1", Table: 100},
						},
						RoutingPolicy: []infrav1alpha2.RoutingPolicySpec{
							{To: "10.10.10.0/24", Table: ptr.To(uint32(100))},
							{From: "172.24.16.0/24", Table: ptr.To(uint32(100))},
						},
					},
				},
			},
		},
	}

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}, IPV6: []string{"2001:db8::2"}}, "net1": {IPV4: []string{"10.0.0.10"}, IPV6: []string{"2001:db8::9"}}}
	createIPPools(t, kubeClient, machineScope)
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.0.0.10")
	createIP6AddressResource(t, kubeClient, machineScope, "net1", "2001:db8::9")

	cfg := &types.NetworkConfigData{Name: "net1"}
	err := getCommonInterfaceConfig(context.Background(), machineScope, cfg, machineScope.ProxmoxMachine.Spec.Network.NetworkDevices[0].InterfaceConfig)
	// TODO: not meaningful anylonger
	/*
		require.Equal(t, "10.0.0.10/24", cfg.IPConfigs[0].IPAddress)
		require.Equal(t, "2001:db8::9/64", cfg.IPConfigs[1].IPAddress)
	*/
	require.Equal(t, "1.2.3.4", cfg.DNSServers[0])
	require.Equal(t, "default", cfg.Routes[0].To)
	/*
		require.Equal(t, "172.24.16.0/24", cfg.Routes[1].To)
	*/
	require.Equal(t, "10.10.10.0/24", cfg.FIBRules[0].To)
	require.Equal(t, "172.24.16.0/24", cfg.FIBRules[1].From)
	require.NoError(t, err)
}

func TestGetVirtualNetworkDevices_VRFDevice_MissingInterface(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.SetVirtualMachine(newStoppedVM())

	networkSpec := infrav1alpha2.NetworkSpec{
		VirtualNetworkDevices: infrav1alpha2.VirtualNetworkDevices{
			VRFs: []infrav1alpha2.VRFDevice{{
				Name:       "vrf-blue",
				Table:      500,
				Interfaces: []string{"net1"},
			}},
		},
	}
	networkConfigData := []types.NetworkConfigData{{}}

	cfg, err := getVirtualNetworkDevices(context.Background(), machineScope, networkSpec, networkConfigData)
	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestReconcileBootstrapData_DualStack(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config = &infrav1alpha2.IPConfigSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}, IPV6: []string{"2001:db8::2"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")
	createIP6AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "2001:db8::2")

	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestReconcileBootstrapData_DualStack_AdditionalDevices(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config = &infrav1alpha2.IPConfigSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha2.NetworkSpec{
		NetworkDevices: []infrav1alpha2.NetworkDevice{
			defaultNic,
			{
				Bridge: "vmbr1",
				Model:  ptr.To("virtio"),
				Name:   "net1",
				InterfaceConfig: infrav1alpha2.InterfaceConfig{
					DNSServers: []string{"1.2.3.4"},
					IPPoolRef: []corev1.TypedLocalObjectReference{{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "GlobalInClusterIPPool",
						Name:     "sample",
					}, {
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "InClusterIPPool",
						Name:     "sample",
					}},
				},
			},
		},
	}

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}, IPV6: []string{"2001:db8::2"}}, "net1": {IPV4: []string{"10.0.0.10"}, IPV6: []string{"2001:db8::9"}}}
	createIPPools(t, kubeClient, machineScope)
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")
	createIP6AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "2001:db8::2")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.0.0.10")
	createIP6AddressResource(t, kubeClient, machineScope, "net1", "2001:db8::9")
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestReconcileBootstrapData_VirtualDevices_VRF(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha2.NetworkSpec{
		VirtualNetworkDevices: infrav1alpha2.VirtualNetworkDevices{
			VRFs: []infrav1alpha2.VRFDevice{{
				Interfaces: []string{"net1"},
				Name:       "vrf-blue",
				Table:      500,
			}},
		},
		NetworkDevices: []infrav1alpha2.NetworkDevice{
			{
				Bridge: "vmbr1",
				Model:  ptr.To("virtio"),
				Name:   "net1",
				InterfaceConfig: infrav1alpha2.InterfaceConfig{
					DNSServers: []string{"1.2.3.4"},
					IPPoolRef: []corev1.TypedLocalObjectReference{{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "InClusterIPPool",
						Name:     "sample",
					}},
				},
			},
		},
	}
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}, "net1": {IPV4: []string{"10.100.10.10"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")
	createIP4AddressResource(t, kubeClient, machineScope, "net1", "10.100.10.10")

	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
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

	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, conditions.IsFalse(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, conditions.GetReason(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition) == infrav1alpha2.CloningFailedReason)
}

func TestReconcileBootstrapDataMissingNetworkConfig(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)

	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}}
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, conditions.IsFalse(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, conditions.GetReason(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition) == infrav1alpha2.WaitingForStaticIPAllocationReason)
}

func TestReconcileBootstrapData_Format_CloudConfig(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)
	machineScope.SetVirtualMachine(vm)

	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	// test defaulting of format to cloud-config
	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)

	data, format, err := getBootstrapData(context.Background(), machineScope)
	require.Equal(t, cloudinit.FormatCloudConfig, ptr.Deref(format, ""))
	require.Equal(t, []byte("data"), data)
	require.Nil(t, err)
}

func TestReconcileBootstrapData_Format_Ignition(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.10.10.10"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.10.10.10")
	createBootstrapSecret(t, kubeClient, machineScope, ignition.FormatIgnition)
	machineScope.SetVirtualMachine(vm)

	getIgnitionISOInjector = func(_ *proxmox.VirtualMachine, _ cloudinit.Renderer, _ *ignition.Enricher) isoInjector {
		return FakeIgnitionISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)

	data, format, err := getBootstrapData(context.Background(), machineScope)
	require.Equal(t, ignition.FormatIgnition, ptr.Deref(format, ""))
	require.Equal(t, []byte("{\"ignition\":{\"version\":\"2.3.0\"}}"), data)
	require.Nil(t, err)
}

func TestDefaultISOInjector(t *testing.T) {
	injector := defaultISOInjector(newRunningVM(), []byte("data"), cloudinit.NewMetadata(biosUUID, "test", "1.2.3", true), cloudinit.NewNetworkConfig(nil))

	require.NotEmpty(t, injector)
	require.Equal(t, []byte("data"), injector.(*inject.ISOInjector).BootstrapData)
}

func TestIgnitionISOInjector(t *testing.T) {
	injector := defaultIgnitionISOInjector(newRunningVM(), cloudinit.NewMetadata(biosUUID, "test", "1.2.3", true), &ignition.Enricher{
		BootstrapData: []byte("data"),
		Hostname:      "test",
	})

	require.NotEmpty(t, injector)
	require.NotNil(t, injector.(*inject.ISOInjector).IgnitionEnricher)
	require.Equal(t, []byte("data"), injector.(*inject.ISOInjector).IgnitionEnricher.BootstrapData)
}

func TestReconcileBootstrapData_DefaultDeviceIPPoolRef(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha2.NetworkSpec{
		NetworkDevices: []infrav1alpha2.NetworkDevice{{
			Bridge: "vmbr0",
			Model:  ptr.To("virtio"),
			InterfaceConfig: infrav1alpha2.InterfaceConfig{
				IPPoolRef: []corev1.TypedLocalObjectReference{{
					APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
					Kind:     "GlobalInClusterIPPool",
					Name:     "sample-shared-pool",
				}},
			},
		}},
	}

	vm := newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]*infrav1alpha2.IPAddresses{infrav1alpha2.DefaultNetworkDevice: {IPV4: []string{"10.5.10.10"}}}
	createIP4AddressResource(t, kubeClient, machineScope, infrav1alpha2.DefaultNetworkDevice, "10.5.10.10")

	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)
	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.False(t, conditions.Has(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}
