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
	"fmt"
	"net/netip"
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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

type FakeISOInjector struct {
	Error error
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
	infraCluster.Status.InClusterIPPoolRef = []corev1.LocalObjectReference{{Name: ipam.InClusterPoolFormat(infraCluster, infrav1.IPV4Format)}}

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

func getIPSuffix(addr string) string {
	suffix := infrav1.DefaultSuffix
	ip := netip.MustParseAddr(addr)
	if ip.Is6() {
		suffix += "6"
	}

	return suffix
}

func createIPAddressResource(t *testing.T, c client.Client, name string, machineScope *scope.MachineScope, ip string, prefix int, pool *corev1.TypedLocalObjectReference) {
	if pool != nil {
		ipAddrClaim := &ipamv1.IPAddressClaim{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "ipam.cluster.x-k8s.io/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
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
			Address: ip,
			Prefix:  prefix,
			Gateway: netip.MustParsePrefix(fmt.Sprintf("%s/%d", ip, prefix)).Addr().Next().String(),
			PoolRef: ptr.Deref(pool, corev1.TypedLocalObjectReference{}),
		},
	}
	require.NoError(t, c.Create(context.Background(), ipAddr))
}

func createIP4AddressResource(t *testing.T, c client.Client, machineScope *scope.MachineScope, device, ip string, pool *corev1.TypedLocalObjectReference) {
	require.Truef(t, netip.MustParseAddr(ip).Is4(), "%s is not a valid ipv4 address", ip)
	poolName := ptr.Deref(pool, corev1.TypedLocalObjectReference{Name: "dummy"}).Name
	name := formatIPAddressName(machineScope.Name(), poolName, device)
	name = fmt.Sprintf("%s-%s", name, getIPSuffix(ip))

	createIPAddressResource(t, c, name, machineScope, ip, 24, pool)
}

func createIP6AddressResource(t *testing.T, c client.Client, machineScope *scope.MachineScope, device, ip string, pool *corev1.TypedLocalObjectReference) {
	require.Truef(t, netip.MustParseAddr(ip).Is6(), "%s is not a valid ipv6 address", ip)
	poolName := ptr.Deref(pool, corev1.TypedLocalObjectReference{Name: "dummyv6"}).Name
	name := formatIPAddressName(machineScope.Name(), poolName, device)
	name = fmt.Sprintf("%s-%s", name, getIPSuffix(ip))

	createIPAddressResource(t, c, name, machineScope, ip, 64, pool)
}

func createIPPools(t *testing.T, c client.Client, machineScope *scope.MachineScope) {
	for _, dev := range machineScope.ProxmoxMachine.Spec.Network.NetworkDevices {
		for _, poolRef := range dev.IPPoolRef {
			var obj client.Object
			switch poolRef.Kind {
			case "InClusterIPPool":
				obj = &ipamicv1.InClusterIPPool{}
				obj.SetNamespace(machineScope.Namespace())
			case "GlobalInClusterIPPool":
				obj = &ipamicv1.GlobalInClusterIPPool{}
			}
			obj.SetName(poolRef.Name)
			require.NoError(t, c.Create(context.Background(), obj))
		}
	}
}

// todo: ZONES?
func getDefaultPoolRefs(machineScope *scope.MachineScope) []corev1.LocalObjectReference {
	cluster := machineScope.InfraCluster.ProxmoxCluster

	return cluster.Status.InClusterIPPoolRef
}

func getIPAddressClaims(t *testing.T, c client.Client, machineScope *scope.MachineScope) map[string]*[]ipamv1.IPAddressClaim {
	ipAddressClaims := &ipamv1.IPAddressClaimList{}

	fieldSelector, _ := fields.ParseSelector("ipaddressclaim.ownerMachine=" + machineScope.Name())

	listOptions := client.ListOptions{FieldSelector: fieldSelector}
	c.List(context.Background(), ipAddressClaims, &listOptions)

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
