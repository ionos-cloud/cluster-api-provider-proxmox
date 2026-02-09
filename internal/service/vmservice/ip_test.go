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
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	. "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/consts"
)

const ipTag = "ip_net0_10.10.10.10"

// TestReconcileIPAddresses_CreateDefaultClaim tests if the cluster provided InclusterIPPool IPAddressClaim gets created.
func TestReconcileIPAddresses_CreateDefaultClaim(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForStaticIPAllocationReason)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Name:        ptr.To("net0"),
				DefaultIPv4: ptr.To(true),
			},
		},
	}

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	defaultPoolRefs := getDefaultPoolRefs(machineScope)
	// test if IPAddressClaim was created
	claimsDefaultPool := getIPAddressClaimsPerPool(t, kubeClient, machineScope, defaultPoolRefs.InClusterIPPoolRefV4.Name)
	require.NotNil(t, claimsDefaultPool)
	require.Equal(t, 1, len(*claimsDefaultPool))

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

// TestReconcileIPAddresses_CreateAdditionalClaim tests if an IPAddressClaim is created for the missing IPAddress on net1.
func TestReconcileIPAddresses_CreateAdditionalClaim(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForStaticIPAllocationReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	extraPool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "extraPool0",
	}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{Name: ptr.To("net0")},
			{
				Name: ptr.To("net1"),
				InterfaceConfig: infrav1.InterfaceConfig{
					IPPoolRef: []corev1.TypedLocalObjectReference{extraPool0},
				},
			},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)

	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)

	// Since an IPAddress for extraPool0 still needs to be created, the machine should
	// requeue without error.
	require.NoError(t, err)
	require.True(t, requeue)

	// net1 should not exist yet, because IPAddress reconciliation should be unfinished
	require.Nil(t, machineScope.ProxmoxMachine.GetIPAddressesNet("net1"))

	// test if IPAddressClaim was created
	claimsExtraPool0 := getIPAddressClaimsPerPool(t, kubeClient, machineScope, extraPool0.Name)
	require.NotNil(t, claimsExtraPool0)
	require.Equal(t, 1, len(*claimsExtraPool0))

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

// TestReconcileIPAddresses_AddIPTag tests if a machine with all resources created will add a task to add tags to proxmox VMs.
func TestReconcileIPAddresses_AddIPTag(t *testing.T) {
	machineScope, proxmoxClient, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForStaticIPAllocationReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Name:        ptr.To("net0"),
				DefaultIPv4: ptr.To(true),
			},
		},
	}

	vm := newStoppedVM()
	task := newTask()
	machineScope.SetVirtualMachine(vm)

	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)

	proxmoxClient.EXPECT().TagVM(context.Background(), vm, ipTag).Return(task, nil).Once()

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	// Machine should have one Task Pending
	require.NotNil(t, machineScope.ProxmoxMachine.Status.TaskRef)

	// Task should be equal to fake result from TagVM
	require.Equal(t, "result", *machineScope.ProxmoxMachine.Status.TaskRef)

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

// TestReconcileIPAddresses_SetIPAddresses tests if proxmoxMachine.Status.IPAddresses gets reconciled.
func TestReconcileIPAddresses_SetIPAddresses(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForStaticIPAllocationReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	extraPool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "extraPool0",
	}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Name:        ptr.To("net0"),
				DefaultIPv4: ptr.To(true),
			},
			{
				Name: ptr.To("net1"),
				InterfaceConfig: infrav1.InterfaceConfig{
					IPPoolRef: []corev1.TypedLocalObjectReference{extraPool0},
				},
			},
		},
	}
	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)
	createIPAddress(t, kubeClient, machineScope, "net1", "10.100.10.10", 0, &extraPool0)

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)

	require.NoError(t, err)
	require.True(t, requeue)

	require.NotNil(t, machineScope.ProxmoxMachine.GetIPAddressesNet("net0"))
	require.Equal(
		t,
		infrav1.IPAddressesSpec{NetName: infrav1.DefaultNetworkDevice, IPv4: []string{"10.10.10.10"}, IPv6: nil},
		*machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice),
	)
	require.NotNil(t, machineScope.ProxmoxMachine.GetIPAddressesNet("net1"))
	require.Equal(
		t,
		infrav1.IPAddressesSpec{NetName: "net1", IPv4: []string{"10.100.10.10"}, IPv6: nil},
		*machineScope.ProxmoxMachine.GetIPAddressesNet("net1"),
	)

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

// TestReconcileIPAddresses_MultipleDevices tests if proxmoxMachine.Status.IPAddresses gets reconciled with IPv4 and IPv6 on multiple devices.
func TestReconcileIPAddresses_MultipleDevices(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForStaticIPAllocationReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	ipv4pool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "ipv4pool0",
	}
	ipv4pool1 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "ipv4pool1",
	}
	ipv6pool := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "ipv6pool",
	}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Name:            ptr.To(infrav1.DefaultNetworkDevice),
				DefaultIPv4:     ptr.To(true),
				InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{ipv4pool0}},
			},
			{
				Name:            ptr.To("net1"),
				InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{ipv4pool1}},
			},
			{
				Name:            ptr.To("net2"),
				InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{ipv6pool}},
			},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)

	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.11.10.10", 1, &ipv4pool0)
	createIPAddress(t, kubeClient, machineScope, "net1", "10.100.10.10", 0, &ipv4pool1)
	createIPAddress(t, kubeClient, machineScope, "net2", "fe80::ffee", 0, &ipv6pool)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	// Add one for default network devices
	require.Len(t, machineScope.ProxmoxMachine.GetIPAddresses(), 3+1)

	require.Equal(t,
		[]string{"10.10.10.10", "10.11.10.10"},
		machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice).IPv4,
	)
	require.Nil(t, machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice).IPv6)

	require.ElementsMatch(t,
		[]string{"10.100.10.10"},
		machineScope.ProxmoxMachine.GetIPAddressesNet("net1").IPv4,
	)
	require.Nil(t, machineScope.ProxmoxMachine.GetIPAddressesNet("net1").IPv6)

	require.Nil(t, machineScope.ProxmoxMachine.GetIPAddressesNet("net2").IPv4)
	require.ElementsMatch(t,
		[]string{"fe80::ffee"},
		machineScope.ProxmoxMachine.GetIPAddressesNet("net2").IPv6,
	)

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

// TestReconcileIPAddresses_IPv6 tests if proxmoxMachine.Status.IPAddresses gets reconciled with IPv4 and IPv6 on multiple devices.
func TestReconcileIPAddresses_IPv6(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForStaticIPAllocationReason)

	// add test-v6-icip InClusterIPPool
	proxmoxCluster := machineScope.InfraCluster.ProxmoxCluster
	proxmoxCluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
		Addresses: []string{"fe80::"},
		Prefix:    64,
		Gateway:   "fe80::",
		Metric:    nil,
	}
	require.NoError(t, kubeClient.Update(context.Background(), proxmoxCluster))

	// Status can't be updated and needs to be patched
	patch := client.MergeFrom(proxmoxCluster.DeepCopy())
	proxmoxCluster.Status.InClusterIPPoolRef = []corev1.LocalObjectReference{
		{Name: "test-v4-icip"},
		{Name: "test-v6-icip"},
	}
	proxmoxCluster.Status.InClusterZoneRef = []infrav1.InClusterZoneRef{{
		Zone:                 ptr.To("default"),
		InClusterIPPoolRefV4: ptr.To(corev1.LocalObjectReference{Name: "test-v4-icip"}),
		InClusterIPPoolRefV6: ptr.To(corev1.LocalObjectReference{Name: "test-v6-icip"}),
	}}
	require.NoError(t, kubeClient.Status().Patch(context.Background(), proxmoxCluster, patch))

	// create the extra ipv6 pool
	require.NoError(t, machineScope.IPAMHelper.CreateOrUpdateInClusterIPPool(context.Background()))

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	defaultPoolV6 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: InClusterIPPool,
		Name: getDefaultPoolRefs(machineScope).InClusterIPPoolRefV6.Name,
	}
	extraPool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "extrapool0",
	}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Name:        ptr.To("net0"),
				DefaultIPv4: ptr.To(true),
				DefaultIPv6: ptr.To(true),
			},
			{
				Name:            ptr.To("net1"),
				InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{extraPool0}},
			},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)
	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "fe80::1", 1, &defaultPoolV6)
	createIPAddress(t, kubeClient, machineScope, "net1", "10.100.10.10", 0, &extraPool0)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	// need to reconcile twice for the ipv6 default pool to be added
	requeue, err = reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)

	require.NotNil(t, machineScope.ProxmoxMachine.GetIPAddressesNet("net0"))
	require.Equal(t,
		infrav1.IPAddressesSpec{NetName: infrav1.DefaultNetworkDevice, IPv4: []string{"10.10.10.10"}, IPv6: []string{"fe80::1"}},
		*machineScope.ProxmoxMachine.GetIPAddressesNet("net0"),
	)
	require.NotNil(t, machineScope.ProxmoxMachine.GetIPAddressesNet("net1"))
	require.Equal(t,
		infrav1.IPAddressesSpec{NetName: "net1", IPv4: []string{"10.100.10.10"}, IPv6: nil},
		*machineScope.ProxmoxMachine.GetIPAddressesNet("net1"),
	)

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}

// TestReconcileIPAddresses_MachineIPPoolRef tests.
func TestReconcileIPAddresses_MachineIPPoolRef(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.WaitingForStaticIPAllocationReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	extraPool0 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "extrapool0",
	}
	extraPool1 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "extrapool1",
	}
	extraPool2 := corev1.TypedLocalObjectReference{APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind: GlobalInClusterIPPool,
		Name: "extrapool2",
	}

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Name:            ptr.To("net0"),
				DefaultIPv4:     ptr.To(true),
				InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{extraPool0, extraPool0}},
			},
			{
				Name:            ptr.To("net1"),
				InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{extraPool1, extraPool2, extraPool1, extraPool2}},
			},
		},
	}

	vm := newStoppedVM()
	vm.VirtualMachineConfig.Tags = ipTag
	machineScope.SetVirtualMachine(vm)

	defaultIP := "10.10.10.10"
	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, defaultIP, 0, &defaultPool)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.50.10.10", 1, &extraPool0)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.50.10.11", 2, &extraPool0)
	createIPAddress(t, kubeClient, machineScope, "net1", "10.100.10.10", 0, &extraPool1)
	createIPAddress(t, kubeClient, machineScope, "net1", "2001:db8::1", 1, &extraPool2)
	createIPAddress(t, kubeClient, machineScope, "net1", "10.100.10.11", 2, &extraPool1)
	createIPAddress(t, kubeClient, machineScope, "net1", "2001:db8::2", 3, &extraPool2)

	requeue, err := reconcileIPAddresses(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	require.NotNil(t, machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice))
	require.Equal(t,
		[]string{defaultIP, "10.50.10.10", "10.50.10.11"},
		machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice).IPv4,
	)
	require.Nil(t, machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice).IPv6)
	require.NotNil(t, machineScope.ProxmoxMachine.GetIPAddressesNet("net1"))
	require.Equal(t,
		infrav1.IPAddressesSpec{NetName: "net1", IPv4: []string{"10.100.10.10", "10.100.10.11"}, IPv6: []string{"2001:db8::1", "2001:db8::2"}},
		*machineScope.ProxmoxMachine.GetIPAddressesNet("net1"),
	)

	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition)
}
