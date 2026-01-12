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
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/util/conditions"

	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	. "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/consts"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
)

const (
	biosUUID = "uuid=41ec1197-580f-460b-b41b-1dfefabe6e32"
)

var defaultNic = infrav1.NetworkDevice{
	Bridge: ptr.To("vmbr0"),
	Model:  ptr.To("virtio"),
	Name:   ptr.To(infrav1.DefaultNetworkDevice),
}

func setupFakeIsoInjector(t *testing.T) *[]byte {
	networkData := new([]byte)
	getISOInjector = func(vm *proxmox.VirtualMachine, bootstrapData []byte, metadata, network cloudinit.Renderer) isoInjector {
		*networkData, _ = network.Inspect()
		return FakeISOInjector{
			VirtualMachine: vm,
			BootstrapData:  bootstrapData,
			MetaData:       metadata,
			Network:        network,
		}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	return networkData
}

func setupVMWithMetadata(t *testing.T, machineScope *scope.MachineScope, netSpecs ...string) *proxmox.VirtualMachine {
	if len(netSpecs) == 0 {
		netSpecs = []string{"virtio=A6:23:64:4D:84:CB,bridge=vmbr0"}
	}
	vm := newVMWithNets(netSpecs[0], netSpecs[1:]...)
	vm.VirtualMachineConfig.SMBios1 = biosUUID
	machineScope.SetVirtualMachine(vm)

	return vm
}

func addDefaultIPPool(machineScope *scope.MachineScope) corev1.TypedLocalObjectReference {
	poolRefs := getDefaultPoolRefs(machineScope)
	proxmoxCluster := machineScope.InfraCluster.ProxmoxCluster
	if index := slices.IndexFunc(poolRefs, func(p corev1.LocalObjectReference) bool {
		return strings.Contains(p.Name, "v4-icip")
	}); index < 0 {
		proxmoxCluster.Status.InClusterIPPoolRef = append(proxmoxCluster.Status.InClusterIPPoolRef,
			corev1.LocalObjectReference{Name: "test-v4-icip"})
	}

	defaultPool := corev1.TypedLocalObjectReference{APIGroup: GetIpamInClusterAPIGroup(),
		Kind: GetInClusterIPPoolKind(),
		Name: getDefaultPoolRefs(machineScope)[0].Name,
	}
	// call to add defaultNic
	addIPPool(machineScope, defaultPool, ptr.To(infrav1.DefaultNetworkDevice))
	return defaultPool
}

func addDefaultIPPoolV6(machineScope *scope.MachineScope) corev1.TypedLocalObjectReference {
	poolRefs := getDefaultPoolRefs(machineScope)
	proxmoxCluster := machineScope.InfraCluster.ProxmoxCluster
	if index := slices.IndexFunc(poolRefs, func(p corev1.LocalObjectReference) bool {
		return strings.Contains(p.Name, "v6-icip")
	}); index < 0 {
		proxmoxCluster.Status.InClusterIPPoolRef = append(proxmoxCluster.Status.InClusterIPPoolRef,
			corev1.LocalObjectReference{Name: "test-v6-icip"})
	}
	defaultPoolV6 := corev1.TypedLocalObjectReference{APIGroup: GetIpamInClusterAPIGroup(),
		Kind: GetInClusterIPPoolKind(),
		Name: getDefaultPoolRefs(machineScope)[1].Name,
	}
	// call to add defaultNic
	addIPPool(machineScope, defaultPoolV6, ptr.To(infrav1.DefaultNetworkDevice))
	return defaultPoolV6
}

func addInClusterIPPool(machineScope *scope.MachineScope, poolName string, netName infrav1.NetName) corev1.TypedLocalObjectReference {
	inClusterIPPool := corev1.TypedLocalObjectReference{APIGroup: GetIpamInClusterAPIGroup(),
		Kind: GetInClusterIPPoolKind(),
		Name: poolName,
	}
	addIPPool(machineScope, inClusterIPPool, netName)
	return inClusterIPPool
}

func addGlobalInClusterIPPool(machineScope *scope.MachineScope, poolName string, netName infrav1.NetName) corev1.TypedLocalObjectReference {
	globalInClusterIPPool := corev1.TypedLocalObjectReference{APIGroup: GetIpamInClusterAPIGroup(),
		Kind: GetGlobalInClusterIPPoolKind(),
		Name: poolName,
	}
	addIPPool(machineScope, globalInClusterIPPool, netName)
	return globalInClusterIPPool
}

// adds IPPool to ProxmoxMachine, and a containing network interface if it does not exist
func addIPPool(machineScope *scope.MachineScope, poolRef corev1.TypedLocalObjectReference, netName infrav1.NetName) {
	networkSpec := ptr.Deref(machineScope.ProxmoxMachine.Spec.Network, infrav1.NetworkSpec{})
	networkDevices := networkSpec.NetworkDevices

	// check if NetworkDevice even exists, if it doesn't, add it
	var index int
	if index = slices.IndexFunc(networkDevices, func(e infrav1.NetworkDevice) bool { return reflect.DeepEqual(e.Name, netName) }); index < 0 {
		index = len(networkDevices)
		nic := infrav1.NetworkDevice{
			Bridge: ptr.To(fmt.Sprintf("vmbr%d", index)),
			Model:  ptr.To("virtio"),
			Name:   netName,
			InterfaceConfig: infrav1.InterfaceConfig{
				IPPoolRef:  []corev1.TypedLocalObjectReference{},
				DNSServers: []string{fmt.Sprintf("1.2.3.%d", 4+index)},
			},
		}
		networkDevices = append(networkDevices, nic)
	}

	// Add IPPoolRef unless we're dealing with default pools here
	ipPoolRefs := networkDevices[index].InterfaceConfig.IPPoolRef
	if !slices.ContainsFunc(getDefaultPoolRefs(machineScope), func(c corev1.LocalObjectReference) bool {
		return c.Name == poolRef.Name
	}) {
		ipPoolRefs = append(ipPoolRefs, poolRef)
	}

	networkDevices[index].InterfaceConfig.IPPoolRef = ipPoolRefs
	networkSpec.NetworkDevices = networkDevices
	machineScope.ProxmoxMachine.Spec.Network = &networkSpec
}

// TestReconcileBootsrapData_NoNetworkConfig_UpdateStatus tests the simplest setup
// with only a default network pool being reconciled correctly.
func TestReconcileBootstrapData_NoNetworkConfig_UpdateStatus(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)

	// setup bootstrapdata injection fake
	networkDataPtr := setupFakeIsoInjector(t)

	// setup VM with all metadata
	setupVMWithMetadata(t, machineScope)
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	// NetworkSetup
	// defaultPool addresses need to be created individually since they refer to no interface atm
	defaultPool := addDefaultIPPool(machineScope)
	createIPAddress(t, kubeClient, machineScope, "net0", "10.10.10.10/24", 0, &defaultPool)

	// reconcile BootstrapData
	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)

	// Check generated bootstrapData against setup
	networkConfigData := getNetworkConfigDataFromVM(t, *networkDataPtr)
	require.Equal(t, 1, len(networkConfigData))
	require.Equal(t, "10.10.10.10"+"/24", networkConfigData[0].IPConfigs[0].IPAddress)
	require.Equal(t, "A6:23:64:4D:84:CB", networkConfigData[0].MacAddress)
	require.Equal(t, "eth0", networkConfigData[0].Name)
	require.Equal(t, "net0", *networkConfigData[0].ProxName)
	require.Equal(t, "ethernet", networkConfigData[0].Type)

	require.Equal(t, infrav1.WaitingForVMPowerUpReason, conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

// TestReconcileBootstrapData_UpdateStatus.
func TestReconcileBootstrapData_UpdateStatus(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)

	// setup bootstrapdata injection fake
	networkDataPtr := setupFakeIsoInjector(t)

	// NetworkSetup
	defaultPool := addDefaultIPPool(machineScope)
	extraPool0 := addGlobalInClusterIPPool(machineScope, "extraPool0", ptr.To("net1"))

	createIPPools(t, kubeClient, machineScope)

	// update extraPool for gateway/prefix test
	poolObj := getIPAddressPool(t, kubeClient, machineScope, &extraPool0)
	poolObj.(*ipamicv1.GlobalInClusterIPPool).Spec.Prefix = 16
	poolObj.(*ipamicv1.GlobalInClusterIPPool).Spec.Gateway = "10.100.10.1"
	createOrUpdateIPPool(t, kubeClient, machineScope, nil, poolObj)

	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10/24", 0, &defaultPool)
	createIPAddress(t, kubeClient, machineScope, "net1", "10.100.10.10", 0, &extraPool0)

	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	// reconcile BootstrapData
	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)

	// Check generated bootstrapData against setup
	networkConfigData := getNetworkConfigDataFromVM(t, *networkDataPtr)
	require.True(t, len(networkConfigData) > 0)
	require.Equal(t, "10.10.10.10"+"/24", networkConfigData[0].IPConfigs[0].IPAddress)
	require.Equal(t, "A6:23:64:4D:84:CB", networkConfigData[0].MacAddress)
	require.Equal(t, "eth0", networkConfigData[0].Name)
	require.Equal(t, "net0", *networkConfigData[0].ProxName)
	require.Equal(t, "ethernet", networkConfigData[0].Type)
	require.Equal(t, "10.100.10.10"+"/16", networkConfigData[1].IPConfigs[0].IPAddress)
	require.Equal(t, "10.100.10.1", networkConfigData[1].IPConfigs[0].Gateway)
	require.Equal(t, "AA:23:64:4D:84:CD", networkConfigData[1].MacAddress)
	require.Equal(t, "eth1", networkConfigData[1].Name)
	require.Equal(t, "net1", *networkConfigData[1].ProxName)
	require.Equal(t, "ethernet", networkConfigData[1].Type)

	require.Equal(t, infrav1.WaitingForVMPowerUpReason, conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

// TestReconcileBootstrapData_BadInjector is supposed to fail when rendering VM configuration data.
func TestReconcileBootstrapData_BadInjector(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope)
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	// NetworkSetup
	defaultPool := addDefaultIPPool(machineScope)

	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)

	getISOInjector = func(_ *proxmox.VirtualMachine, _ []byte, _, _ cloudinit.Renderer) isoInjector {
		return FakeISOInjector{Error: errors.New("bad FakeISOInjector")}
	}
	t.Cleanup(func() { getISOInjector = defaultISOInjector })

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to inject bootstrap data: bad FakeISOInjector")
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.Nil(t, machineScope.ProxmoxMachine.Status.BootstrapDataProvided)
}

func TestGetBootstrapData_MissingSecretName(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)

	data, _, err := getBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.Equal(t, err.Error(), "machine has no bootstrap data")
	require.Nil(t, data)
}

func TestGetBootstrapData_MissingSecretNotName(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)

	machineScope.Machine.Spec.Bootstrap.DataSecretName = ptr.To("foo")
	data, _, err := getBootstrapData(context.Background(), machineScope)

	require.Error(t, err)
	require.Equal(t, err.Error(), "failed to retrieve bootstrap data secret: secrets \"foo\" not found")
	require.Nil(t, data)
}

func TestGetBootstrapData_MissingSecretValue(t *testing.T) {
	machineScope, _, client := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)

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

func TestGetNetworkConfigDataForDevice_MissingIPAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope)

	cfg, err := getNetworkConfigDataForDevice(context.Background(), machineScope, "net0", nil)
	require.NoError(t, err)
	require.Equal(t, cfg.MacAddress, "A6:23:64:4D:84:CB")
	require.Len(t, cfg.IPConfigs, 0)
}

func TestGetNetworkConfigDataForDevice_MissingMACAddress(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	machineScope.SetVirtualMachine(newStoppedVM())

	cfg, err := getNetworkConfigDataForDevice(context.Background(), machineScope, "net2", nil)
	require.Error(t, err)
	require.Equal(t, "unable to extract mac address", err.Error())
	require.Nil(t, cfg)
}

func TestGetCommonInterfaceConfig_MissingIPPool(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			defaultNic,
			{
				Bridge: ptr.To("vmbr1"),
				Model:  ptr.To("virtio"),
				Name:   ptr.To("net1"),
				InterfaceConfig: infrav1.InterfaceConfig{
					IPPoolRef: []corev1.TypedLocalObjectReference{{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     GlobalInClusterIPPool,
						Name:     "net1-inet",
					}},
				},
			},
		},
	}

	cfg := &types.NetworkConfigData{Name: "net1"}
	getCommonInterfaceConfig(context.Background(), machineScope, cfg, machineScope.ProxmoxMachine.Spec.Network.NetworkDevices[0].InterfaceConfig)
	// Check that no IP config has been assigned even in the presence of an IPPoolRef.
	require.Len(t, cfg.IPConfigs, 0)
}

func TestGetCommonInterfaceConfig(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)

	var MTU int32 = 9000
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Bridge: ptr.To("vmbr1"),
				Model:  ptr.To("virtio"),
				Name:   ptr.To("net1"),
				InterfaceConfig: infrav1.InterfaceConfig{
					DNSServers: []string{"1.2.3.4"},
					LinkMTU:    &MTU,
					Routing: infrav1.Routing{
						Routes: []infrav1.RouteSpec{
							{To: ptr.To("default"), Via: ptr.To("192.168.178.1")},
							{To: ptr.To("172.24.16.0/24"), Via: ptr.To("192.168.178.1"), Table: ptr.To(int32(100))},
						},
						RoutingPolicy: []infrav1.RoutingPolicySpec{
							{To: ptr.To("10.10.10.0/24"), Table: ptr.To(int32(100))},
							{From: ptr.To("172.24.16.0/24"), Table: ptr.To(int32(100))},
						},
					},
				},
			},
		},
	}

	cfg := &types.NetworkConfigData{Name: "net1"}
	getCommonInterfaceConfig(context.Background(), machineScope, cfg, machineScope.ProxmoxMachine.Spec.Network.NetworkDevices[0].InterfaceConfig)
	require.Equal(t, "1.2.3.4", cfg.DNSServers[0])
	require.Equal(t, "default", *cfg.Routes[0].To)
	require.Equal(t, "172.24.16.0/24", *cfg.Routes[1].To)
	require.Equal(t, "10.10.10.0/24", *cfg.FIBRules[0].To)
	require.Equal(t, "172.24.16.0/24", *cfg.FIBRules[1].From)
}

func TestGetVirtualNetworkDevices_VRFDevice_MissingInterface(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	machineScope.SetVirtualMachine(newStoppedVM())

	networkSpec := infrav1.NetworkSpec{
		VirtualNetworkDevices: infrav1.VirtualNetworkDevices{
			VRFs: []infrav1.VRFDevice{{
				Name:       "vrf-blue",
				Table:      500,
				Interfaces: []infrav1.NetName{ptr.To("net1")},
			}},
		},
	}
	networkConfigData := []types.NetworkConfigData{{}}

	cfg, err := getVirtualNetworkDevices(context.Background(), machineScope, networkSpec, networkConfigData)
	require.Error(t, err)
	require.Equal(t, "unable to find vrf interface=vrf-blue child interface net1", err.Error())
	require.Nil(t, cfg)
}

func TestReconcileBootstrapData_DualStack(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	networkDataPtr := setupFakeIsoInjector(t)
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	// ProxmoxCluster needs IPv6 pool added
	proxmoxCluster := machineScope.InfraCluster.ProxmoxCluster
	proxmoxCluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
		Addresses: []string{"2001:db8::/96"},
		Prefix:    96,
		Gateway:   "2001:db8::1",
	}
	require.NoError(t, kubeClient.Update(context.Background(), proxmoxCluster))
	proxmoxCluster.Status.InClusterIPPoolRef = []corev1.LocalObjectReference{
		{Name: "test-v4-icip"},
		{Name: "test-v6-icip"},
	}
	require.NoError(t, kubeClient.Status().Update(context.Background(), proxmoxCluster))

	// NetworkSetup for default pools
	defaultPool := addDefaultIPPool(machineScope)
	defaultPoolV6 := addDefaultIPPoolV6(machineScope)

	// create missing defaultPoolV6 and ipAddresses
	require.NoError(t, machineScope.IPAMHelper.CreateOrUpdateInClusterIPPool(context.Background()))
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.0.0.254", 0, &defaultPool)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "2001:db8::2", 0, &defaultPoolV6)

	// perform test
	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.Equal(t, infrav1.WaitingForVMPowerUpReason, conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)

	// Test if generated data is equal
	networkConfigData := getNetworkConfigDataFromVM(t, *networkDataPtr)
	require.True(t, len(networkConfigData) == 1)
	require.True(t, len(networkConfigData[0].IPConfigs) == 2)
	ipConfigs := networkConfigData[0].IPConfigs
	require.Equal(t, "10.0.0.1", ipConfigs[0].Gateway)
	require.Equal(t, "10.0.0.254/24", ipConfigs[0].IPAddress)
	require.Equal(t, "2001:db8::1", ipConfigs[1].Gateway)
	require.Equal(t, "2001:db8::2/96", ipConfigs[1].IPAddress)
}

func TestReconcileBootstrapData_DualStack_AdditionalDevices(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	networkDataPtr := setupFakeIsoInjector(t)
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	proxmoxCluster := machineScope.InfraCluster.ProxmoxCluster
	proxmoxCluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
		Addresses: []string{"2001:db8::/96"},
		Prefix:    96,
		Gateway:   "2001:db8::1",
	}
	require.NoError(t, kubeClient.Update(context.Background(), proxmoxCluster))
	proxmoxCluster.Status.InClusterIPPoolRef = []corev1.LocalObjectReference{
		{Name: "test-v4-icip"},
		{Name: "test-v6-icip"},
	}
	require.NoError(t, kubeClient.Status().Update(context.Background(), proxmoxCluster))

	// create missing defaultPoolV6.
	require.NoError(t, machineScope.IPAMHelper.CreateOrUpdateInClusterIPPool(context.Background()))

	// NetworkSetup for default pools.
	addDefaultIPPool(machineScope)
	addDefaultIPPoolV6(machineScope)

	// NetworkSetup for extra pools.
	addGlobalInClusterIPPool(machineScope, "extraPool0", ptr.To("net1"))
	addGlobalInClusterIPPool(machineScope, "extraPool1", ptr.To("net1"))

	// Create missing ip addresses and pools.
	createNetworkSpecForMachine(t, kubeClient, machineScope, "10.10.10.10", "2001:db8::2", "10.0.0.10", "2001:db8::9")

	// Perform test.
	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.Equal(t, infrav1.WaitingForVMPowerUpReason, conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)

	// Test if generated data is equal.
	networkConfigData := getNetworkConfigDataFromVM(t, *networkDataPtr)
	require.Equal(t, 2, len(networkConfigData))
	require.Equal(t, 2, len(networkConfigData[0].IPConfigs))
	require.Equal(t, 2, len(networkConfigData[1].IPConfigs))
	ipConfigs := networkConfigData[0].IPConfigs
	require.Equal(t, "10.0.0.1", ipConfigs[0].Gateway)
	require.Equal(t, "10.10.10.10/24", ipConfigs[0].IPAddress)
	require.Equal(t, "2001:db8::1", ipConfigs[1].Gateway)
	require.Equal(t, "2001:db8::2/96", ipConfigs[1].IPAddress)
	ipConfigs = networkConfigData[1].IPConfigs
	require.Equal(t, "", ipConfigs[0].Gateway) // No Gateway assigned
	require.Equal(t, "10.0.0.10/24", ipConfigs[0].IPAddress)
	require.Equal(t, "", ipConfigs[1].Gateway) // No Gateway assigned
	require.Equal(t, "2001:db8::9/64", ipConfigs[1].IPAddress)
}

func TestReconcileBootstrapData_VirtualDevices_VRF(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0", "virtio=AA:23:64:4D:84:CD,bridge=vmbr1")
	networkDataPtr := setupFakeIsoInjector(t)
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		DefaultNetworkSpec: infrav1.DefaultNetworkSpec{
			ClusterPoolDeviceV4: ptr.To("net0"),
			ClusterPoolDeviceV6: ptr.To("net0"),
		},
		VirtualNetworkDevices: infrav1.VirtualNetworkDevices{
			VRFs: []infrav1.VRFDevice{{
				Interfaces: []infrav1.NetName{ptr.To("net1")},
				Name:       "vrf-blue",
				Table:      500,
			}},
		},
	}

	// NetworkSetup for default pools
	addDefaultIPPool(machineScope)

	// NetworkSetup for extra pools.
	addGlobalInClusterIPPool(machineScope, "extraPool0", ptr.To("net0"))
	addGlobalInClusterIPPool(machineScope, "extraPool1", ptr.To("net1"))

	createNetworkSpecForMachine(t, kubeClient, machineScope, "10.10.10.10", "10.20.10.10/23", "10.100.10.10/22")

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)

	// Test if generated data is equal.
	networkConfigData := getNetworkConfigDataFromVM(t, *networkDataPtr)
	require.Equal(t, 3, len(networkConfigData))
	require.Equal(t, 2, len(networkConfigData[0].IPConfigs))
	require.Equal(t, 1, len(networkConfigData[1].IPConfigs))
	require.Equal(t, 0, len(networkConfigData[2].IPConfigs))
	require.Equal(t, 1, len(networkConfigData[2].Interfaces))
	ipConfigs := networkConfigData[0].IPConfigs
	require.Equal(t, "10.0.0.1", ipConfigs[0].Gateway)
	require.Equal(t, "10.10.10.10/24", ipConfigs[0].IPAddress)
	require.Equal(t, "10.20.10.10/23", ipConfigs[1].IPAddress)
	ipConfigs = networkConfigData[1].IPConfigs
	require.Equal(t, "10.100.10.10/22", ipConfigs[0].IPAddress)
	// VRF Data
	require.Equal(t, "vrf", networkConfigData[2].Type)
	require.Equal(t, "vrf-blue", networkConfigData[2].Name)
	require.Equal(t, "eth1", networkConfigData[2].Interfaces[0])
	require.Equal(t, int32(500), networkConfigData[2].Table)
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
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0")

	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0)

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, conditions.IsFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) == infrav1.CloningFailedReason)
}

func TestReconcileBootstrapDataMissingNetworkConfig(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.Error(t, err)
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, conditions.IsFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.Equal(t, infrav1.VMProvisionFailedReason, conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.ErrorContains(t, err, "network config data is not set")
}

func TestReconcileBootstrapData_Format_CloudConfig(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0)

	setupFakeIsoInjector(t)

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
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	createBootstrapSecret(t, kubeClient, machineScope, ignition.FormatIgnition)

	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0)

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
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForBootstrapDataReconcilationReason)
	setupVMWithMetadata(t, machineScope, "virtio=A6:23:64:4D:84:CB,bridge=vmbr0")
	networkDataPtr := setupFakeIsoInjector(t)
	createBootstrapSecret(t, kubeClient, machineScope, cloudinit.FormatCloudConfig)

	// NetworkSetup for default pools
	addDefaultIPPool(machineScope)

	// NetworkSetup for extra pools.
	addGlobalInClusterIPPool(machineScope, "extraPool0", ptr.To("net0"))

	createNetworkSpecForMachine(t, kubeClient, machineScope, "10.10.10.10", "10.5.10.10/23")

	// Perform the test
	requeue, err := reconcileBootstrapData(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
	require.True(t, conditions.Has(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition))
	require.True(t, *machineScope.ProxmoxMachine.Status.BootstrapDataProvided)

	// Test if generated data is equal
	networkConfigData := getNetworkConfigDataFromVM(t, *networkDataPtr)
	require.Equal(t, 1, len(networkConfigData))
	require.Equal(t, 2, len(networkConfigData[0].IPConfigs))
	ipConfigs := networkConfigData[0].IPConfigs
	require.Equal(t, "10.0.0.1", ipConfigs[0].Gateway)
	require.Equal(t, "10.10.10.10/24", ipConfigs[0].IPAddress)
	require.Equal(t, "", ipConfigs[1].Gateway)
	require.Equal(t, "10.5.10.10/23", ipConfigs[1].IPAddress)
}
