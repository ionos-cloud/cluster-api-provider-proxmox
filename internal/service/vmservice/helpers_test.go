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
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"testing"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fields "k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"         //nolint:staticcheck
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"            //nolint:staticcheck
	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions" //nolint:staticcheck
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	. "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/consts"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
)

type FakeISOInjector struct {
	Error          error
	VirtualMachine *proxmox.VirtualMachine
	BootstrapData  []byte
	MetaData       cloudinit.Renderer
	Network        cloudinit.Renderer
}

func (f FakeISOInjector) Inject(_ context.Context, _ inject.BootstrapDataFormat) error {
	return f.Error
}

type FakeIgnitionISOInjector struct {
	Error error
}

func (f FakeIgnitionISOInjector) Inject(_ context.Context, _ inject.BootstrapDataFormat) error {
	return f.Error
}

// setupReconcilerTestWithCondition sets up a reconciler test with a condition for the proxmoxmachiens statemachine.
func setupReconcilerTestWithCondition(t *testing.T, condition string) (*scope.MachineScope, *proxmoxtest.MockClient, client.Client) {
	machineScope, mockClient, client := setupReconcilerTest(t)

	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, condition, clusterv1.ConditionSeverityInfo, "")

	return machineScope, mockClient, client
}

// setupReconcilerTest initializes a MachineScope with a mock Proxmox client and a fake controller-runtime client.
func setupReconcilerTest(t *testing.T) (*scope.MachineScope, *proxmoxtest.MockClient, client.Client) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
		},
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"cluster.x-k8s.io/cluster-name": "test",
			},
		},
	}

	infraCluster := &infrav1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
			Kind:       "ProxmoxCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
			Finalizers: []string{
				infrav1.ClusterFinalizer,
			},
		},
		Spec: infrav1.ProxmoxClusterSpec{
			IPv4Config: &infrav1.IPConfigSpec{
				Addresses: []string{"10.0.0.10-10.0.0.20"},
				Prefix:    24,
				Gateway:   "10.0.0.1",
			},
			DNSServers: []string{"1.2.3.4"},
		},
		Status: infrav1.ProxmoxClusterStatus{
			NodeLocations: &infrav1.NodeLocations{},
		},
	}
	infraCluster.Status.InClusterIPPoolRef = []corev1.LocalObjectReference{{Name: ipam.InClusterPoolFormat(infraCluster, nil, infrav1.IPv4Format)}}
	infraCluster.Status.InClusterZoneRef = []infrav1.InClusterZoneRef{{
		Zone:                 ptr.To("default"),
		InClusterIPPoolRefV4: &corev1.LocalObjectReference{Name: ipam.InClusterPoolFormat(infraCluster, nil, infrav1.IPv4Format)},
	}}

	infraMachine := &infrav1.ProxmoxMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
			Kind:       "ProxmoxMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
			Finalizers: []string{
				infrav1.MachineFinalizer,
			},
			Labels: map[string]string{
				"cluster.x-k8s.io/cluster-name": "test",
			},
		},
		Spec: ptr.To(infrav1.ProxmoxMachineSpec{
			Network: ptr.To(infrav1.NetworkSpec{}),
			VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
				TemplateSource: infrav1.TemplateSource{
					SourceNode: ptr.To("node1"),
					TemplateID: ptr.To[int32](123),
				},
			},
		}),
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1.AddToScheme(scheme))
	require.NoError(t, ipamv1.AddToScheme(scheme))
	require.NoError(t, ipamicv1.AddToScheme(scheme))
	require.NoError(t, infrav1.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, machine, infraCluster, infraMachine).
		WithStatusSubresource(&infrav1.ProxmoxCluster{}, &infrav1.ProxmoxMachine{}).
		Build()

	ipamHelper := ipam.NewHelper(kubeClient, infraCluster)
	logger := logr.Discard()

	mockClient := proxmoxtest.NewMockClient(t)

	// fake indexing tests. TODO: Unify

	indexFunc := func(obj client.Object) []string {
		return []string{obj.(*ipamv1.IPAddress).Spec.PoolRef.Name}
	}

	err := fake.AddIndex(kubeClient, &ipamv1.IPAddress{}, "spec.poolRef.name", indexFunc)
	require.NoError(t, err)

	// set up index for ipAddressClaims owner ProxmoxMachine (testing of interfaces)
	indexFunc = func(obj client.Object) []string {
		var ret = []string{}

		owners := obj.(*ipamv1.IPAddressClaim).ObjectMeta.OwnerReferences

		for _, owner := range owners {
			if owner.Kind == infrav1.ProxmoxMachineKind {
				ret = append(ret, owner.Name)
			}
		}
		return ret
	}

	err = fake.AddIndex(kubeClient, &ipamv1.IPAddressClaim{}, "ipaddressclaim.ownerMachine", indexFunc)
	require.NoError(t, err)

	// Create InClusterIPPools after the indexes are set up
	require.NoError(t, ipamHelper.CreateOrUpdateInClusterIPPool(context.Background()))

	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Client:         kubeClient,
		Logger:         &logger,
		Cluster:        cluster,
		ProxmoxCluster: infraCluster,
		ProxmoxClient:  mockClient,
		IPAMHelper:     ipamHelper,
	})
	require.NoError(t, err)

	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Client:         kubeClient,
		Logger:         &logger,
		Cluster:        cluster,
		Machine:        machine,
		InfraCluster:   clusterScope,
		ProxmoxMachine: infraMachine,
		IPAMHelper:     ipamHelper,
	})
	require.NoError(t, err)

	return machineScope, mockClient, kubeClient
}

func createIPAddressResource(t *testing.T, c client.Client, name string, machineScope *scope.MachineScope, ip netip.Prefix, offset int, pool *corev1.TypedLocalObjectReference) {
	prefix := ip.Bits()
	var gateway string

	if pool != nil {
		ipAddrClaim := &ipamv1.IPAddressClaim{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "ipam.cluster.x-k8s.io/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					infrav1.ProxmoxPoolRefCounterAnnotation: fmt.Sprintf("%d", offset),
				},
				Name:      name,
				Namespace: machineScope.Namespace(),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: machineScope.ProxmoxMachine.APIVersion,
					Kind:       "ProxmoxMachine",
					Name:       machineScope.Name(),
				}},
			},
			Spec: ipamv1.IPAddressClaimSpec{
				PoolRef: *pool,
			},
		}
		require.NoError(t, c.Create(context.Background(), ipAddrClaim))

		poolSpec := getPoolSpec(getIPAddressPool(t, machineScope, *pool))
		if poolSpec.prefix != 0 {
			prefix = poolSpec.prefix
		}
		gateway = poolSpec.gateway
	}

	ipAddr := &ipamv1.IPAddress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "ipam.cluster.x-k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: machineScope.Namespace(),
		},
		Spec: ipamv1.IPAddressSpec{
			Address: ip.Addr().String(),
			Prefix:  prefix,
			Gateway: gateway,
			PoolRef: ptr.Deref(pool, corev1.TypedLocalObjectReference{}),
		},
	}
	require.NoError(t, c.Create(context.Background(), ipAddr))
}

// createIPAddress creates an IP address resource from strings.
// If no pool or nil pool is passed then a dummy pool is used.
// If one objectRefs is passed then it's used as a pool (intended for most tests, typically pass 0 for offset).
// If two objectRefs are passed then the first pool is used for the IP address name and the second for creating
// the IP address resource (intended for createNetworkSpecForMachine, pass your poolRef index for offset).
func createIPAddress(t *testing.T, c client.Client, machineScope *scope.MachineScope, device, ip string, offset int, pool ...*corev1.TypedLocalObjectReference) {
	ipPrefix, err := netip.ParsePrefix(ip)
	if err != nil {
		ipAddr, err := netip.ParseAddr(ip)
		require.NoError(t, err, "%s is not a valid ip address", ip)
		subnet := 24
		if !ipAddr.Is4() {
			subnet = 64
		}
		ipPrefix = netip.PrefixFrom(ipAddr, subnet)
	}

	pools := make([]*corev1.TypedLocalObjectReference, 2)
	copy(pools, pool)
	if pools[1] == nil {
		// yes pools[0] might be nil here anyway and that's ok
		pools[1] = pools[0]
	}

	poolName := ptr.Deref(pools[0], corev1.TypedLocalObjectReference{Name: "dummy"}).Name
	ipName := ipam.IPAddressFormat(machineScope.Name(), &poolName, offset, device)
	createIPAddressResource(t, c, ipName, machineScope, ipPrefix, offset, pools[1])
}

// createNetworkSpecForMachine is a one stop setup. You need to provide the ipPrefixes in order of pools.
func createNetworkSpecForMachine(t *testing.T, c client.Client, machineScope *scope.MachineScope, ipPrefixes ...string) {
	// Can't hurt to create ippools here
	createIPPools(t, c, machineScope)

	defaultPools, _ := machineScope.IPAMHelper.GetInClusterPools(context.Background(), machineScope.ProxmoxMachine)
	i := 0 // counter for ipPrefix variadic argument
	// Create the pools sequentially by ref
	for _, device := range ptr.Deref(machineScope.ProxmoxMachine.Spec.Network, infrav1.NetworkSpec{}).NetworkDevices {
		ipPoolRefs := device.IPPoolRef
		// to do IPv4 first, we need to first append IPv6 in front and then IPv4
		if ptr.Deref(device.DefaultIPv6, false) {
			if defaultPools.IPv6 != nil {
				ipPoolRefs = slices.Concat([]corev1.TypedLocalObjectReference{defaultPools.IPv6.PoolRef}, ipPoolRefs)
			}
		}
		if ptr.Deref(device.DefaultIPv4, false) {
			if defaultPools.IPv4 != nil {
				ipPoolRefs = slices.Concat([]corev1.TypedLocalObjectReference{defaultPools.IPv4.PoolRef}, ipPoolRefs)
			}
		}

		for offset, poolRef := range ipPoolRefs {
			createIPAddress(t, c, machineScope, infrav1.DefaultSuffix, ipPrefixes[i], offset, &corev1.TypedLocalObjectReference{Name: *device.Name}, &poolRef)
			i++
		}
	}
}

func createIPPools(t *testing.T, c client.Client, machineScope *scope.MachineScope) {
	for _, dev := range ptr.Deref(machineScope.ProxmoxMachine.Spec.Network, infrav1.NetworkSpec{}).NetworkDevices {
		for _, poolRef := range dev.IPPoolRef {
			createOrUpdateIPPool(t, c, machineScope, &poolRef, nil)
		}
	}
}

// Justification: if you require the poolRef for further tweaking, returning the objectRef is useful
//
//nolint:unparam
func createOrUpdateIPPool(t *testing.T, c client.Client, machineScope *scope.MachineScope, poolRef *corev1.TypedLocalObjectReference, pool client.Object) *corev1.TypedLocalObjectReference {
	// literally nothing to do
	if pool == nil && poolRef == nil {
		return nil
	}

	if pool == nil {
		switch poolRef.Kind {
		case InClusterIPPool:
			pool = &ipamicv1.InClusterIPPool{TypeMeta: metav1.TypeMeta{Kind: InClusterIPPool, APIVersion: ipamicv1.GroupVersion.String()}}
			pool.SetNamespace(machineScope.Namespace())
		case GlobalInClusterIPPool:
			pool = &ipamicv1.GlobalInClusterIPPool{TypeMeta: metav1.TypeMeta{Kind: GlobalInClusterIPPool, APIVersion: ipamicv1.GroupVersion.String()}}
		}
		pool.SetName(poolRef.Name)
	}

	if poolRef == nil {
		poolRef = &corev1.TypedLocalObjectReference{
			Name:     pool.GetName(),
			Kind:     pool.GetObjectKind().GroupVersionKind().Kind,
			APIGroup: ptr.To(pool.GetObjectKind().GroupVersionKind().Group),
		}
	}

	desired := pool.DeepCopyObject()

	_, err := controllerutil.CreateOrUpdate(context.Background(), c, pool, func() error {
		// TODO: Metric change in annotations
		if pool.GetObjectKind().GroupVersionKind().Kind == InClusterIPPool {
			pool.(*ipamicv1.InClusterIPPool).Spec = desired.(*ipamicv1.InClusterIPPool).Spec
		} else if pool.GetObjectKind().GroupVersionKind().Kind == GlobalInClusterIPPool {
			pool.(*ipamicv1.GlobalInClusterIPPool).Spec = desired.(*ipamicv1.GlobalInClusterIPPool).Spec
		}
		return nil
	},
	)

	require.NoError(t, err)

	return poolRef
}

func getDefaultPoolRefs(machineScope *scope.MachineScope) infrav1.InClusterZoneRef {
	cluster := machineScope.InfraCluster.ProxmoxCluster

	zone := ptr.Deref(machineScope.ProxmoxMachine.Spec.Network.Zone, "default")
	zoneIndex := slices.IndexFunc(cluster.Status.InClusterZoneRef, func(z infrav1.InClusterZoneRef) bool {
		return zone == *z.Zone
	})
	return cluster.Status.InClusterZoneRef[zoneIndex]
}

func getPoolSpec(pool client.Object) struct {
	gateway string
	prefix  int
} {
	var gateway string
	var prefix int
	if pool.GetObjectKind().GroupVersionKind().Kind == InClusterIPPool {
		prefix = pool.(*ipamicv1.InClusterIPPool).Spec.Prefix
		gateway = pool.(*ipamicv1.InClusterIPPool).Spec.Gateway
	} else if pool.GetObjectKind().GroupVersionKind().Kind == GlobalInClusterIPPool {
		prefix = pool.(*ipamicv1.GlobalInClusterIPPool).Spec.Prefix
		gateway = pool.(*ipamicv1.GlobalInClusterIPPool).Spec.Gateway
	}

	return struct {
		gateway string
		prefix  int
	}{gateway: gateway, prefix: prefix}
}

func getIPAddressPool(t *testing.T, machineScope *scope.MachineScope, poolRef corev1.TypedLocalObjectReference) client.Object {
	obj, err := machineScope.IPAMHelper.GetIPPool(context.Background(), poolRef)

	require.NoError(t, err)
	return obj
}

func getIPAddressClaims(t *testing.T, c client.Client, machineScope *scope.MachineScope) map[string]*[]ipamv1.IPAddressClaim {
	ipAddressClaims := &ipamv1.IPAddressClaimList{}

	//TODO: this selector should probably be unified.
	fieldSelector, _ := fields.ParseSelector("ipaddressclaim.ownerMachine=" + machineScope.Name())

	listOptions := client.ListOptions{FieldSelector: fieldSelector}
	require.NoError(t, c.List(context.Background(), ipAddressClaims, &listOptions))

	claimMap := make(map[string]*[]ipamv1.IPAddressClaim)

	for _, claim := range ipAddressClaims.Items {
		pool := claim.Spec.PoolRef.Name

		perPoolClaims := ptr.Deref(claimMap[pool], []ipamv1.IPAddressClaim{})
		perPoolClaims = append(perPoolClaims, claim)
		claimMap[pool] = &perPoolClaims
	}

	return claimMap
}

func getIPAddressClaimsPerPool(t *testing.T, c client.Client, machineScope *scope.MachineScope, pool string) *[]ipamv1.IPAddressClaim {
	ipAddressClaims := getIPAddressClaims(t, c, machineScope)
	return ipAddressClaims[pool]
}

func getNetworkConfigDataFromVM(t *testing.T, jsonData []byte) []types.NetworkConfigData {
	var networkConfigData []types.NetworkConfigData

	err := json.Unmarshal(jsonData, &networkConfigData)

	require.NoError(t, err)

	return networkConfigData
}

func setInClusterIPPoolStatus(scope *scope.MachineScope, poolName string, ipFamily string, zone infrav1.Zone) {
	// Construct fake fake ippool client object to add via API
	var object client.Object
	pool := &ipamicv1.InClusterIPPool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GetIpamInClusterAPIVersion(),
			Kind:       GetInClusterIPPoolKind(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      poolName,
			Namespace: scope.Namespace(),
			Labels: func() map[string]string {
				m := map[string]string{}
				if zone != nil {
					m[infrav1.ProxmoxZoneLabel] = *zone
				}
				return m
			}(),
			Annotations: map[string]string{
				infrav1.ProxmoxIPFamilyAnnotation: ipFamily,
			},
		},
	}

	object = pool
	scope.InfraCluster.ProxmoxCluster.SetInClusterIPPoolRef(object)
}

func createBootstrapSecret(t *testing.T, c client.Client, machineScope *scope.MachineScope, format string) {
	machineScope.Machine.Spec.Bootstrap.DataSecretName = ptr.To(machineScope.Name())
	data := map[string][]byte{}
	switch format {
	case cloudinit.FormatCloudConfig:
		data = map[string][]byte{
			"value":  []byte("data"),
			"format": []byte("cloud-config"),
		}
	case ignition.FormatIgnition:
		data = map[string][]byte{
			"value":  []byte("{\"ignition\":{\"version\":\"2.3.0\"}}"),
			"format": []byte("ignition"),
		}
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machineScope.Name(),
			Namespace: machineScope.Namespace(),
		},
		Data: data,
	}
	require.NoError(t, c.Create(context.Background(), secret))
}

func newTask() *proxmox.Task {
	return &proxmox.Task{UPID: "result"}
}

func newVMResource() *proxmox.ClusterResource {
	return &proxmox.ClusterResource{
		Name: "test",
		Node: "node1",
	}
}

func newRunningVM() *proxmox.VirtualMachine {
	return &proxmox.VirtualMachine{
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{
			Name: "test",
		},
		Name:      "test",
		Node:      "node1",
		Status:    proxmox.StatusVirtualMachineRunning,
		VMID:      123,
		QMPStatus: proxmox.StatusVirtualMachineRunning,
		Template:  false,
	}
}

func newPausedVM() *proxmox.VirtualMachine {
	return &proxmox.VirtualMachine{
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{},
		Name:                 "paused",
		Node:                 "node1",
		Status:               proxmox.StatusVirtualMachineRunning,
		QMPStatus:            proxmox.StatusVirtualMachinePaused,
	}
}

func newStoppedVM() *proxmox.VirtualMachine {
	return &proxmox.VirtualMachine{
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{},
		Name:                 "test",
		Node:                 "node1",
		Status:               proxmox.StatusVirtualMachineStopped,
		QMPStatus:            proxmox.StatusVirtualMachineStopped,
	}
}

func newHibernatedVM() *proxmox.VirtualMachine {
	return &proxmox.VirtualMachine{
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{},
		Name:                 "hibernated",
		Node:                 "node1",
		Status:               proxmox.StatusVirtualMachineStopped,
		QMPStatus:            proxmox.StatusVirtualMachineStopped,
		Lock:                 "suspended",
	}
}

func newVMWithNets(def string, additional ...string) *proxmox.VirtualMachine {
	vm := &proxmox.VirtualMachine{
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{
			Nets: map[string]string{
				"net0": def,
			},
		},
		Name:      "test",
		Node:      "node1",
		Status:    proxmox.StatusVirtualMachineRunning,
		VMID:      123,
		QMPStatus: proxmox.StatusVirtualMachineRunning,
		Template:  false,
	}
	for i, cfg := range additional {
		vm.VirtualMachineConfig.Nets[fmt.Sprintf("net%d", i+1)] = cfg
	}

	return vm
}

// requireConditionIsFalse asserts that the given conditions exists and has status "False".
func requireConditionIsFalse(t *testing.T, getter conditions.Getter, cond clusterv1.ConditionType) {
	t.Helper()
	require.Truef(t, conditions.Has(getter, cond),
		"%T %s does not have condition %v", getter, getter.GetName(), cond)
	require.Truef(t, conditions.IsFalse(getter, cond),
		"expected condition to be %q, got %q", cond, corev1.ConditionFalse, conditions.Get(getter, cond).Status)
}
