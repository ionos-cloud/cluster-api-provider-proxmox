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
	"fmt"
	"net/netip"
	"testing"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

type FakeISOInjector struct {
	error
}

func (f FakeISOInjector) Inject(_ context.Context) error {
	return f.error
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
		},
	}

	infraCluster := &infrav1alpha1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
			Finalizers: []string{
				infrav1alpha1.ClusterFinalizer,
			},
		},
		Spec: infrav1alpha1.ProxmoxClusterSpec{
			IPv4Config: &infrav1alpha1.IPPoolSpec{
				Addresses: []string{"10.0.0.10-10.0.0.20"},
				Prefix:    24,
				Gateway:   "10.0.0.1",
			},
			DNSServers: []string{"1.2.3.4"},
		},
		Status: infrav1alpha1.ProxmoxClusterStatus{
			NodeLocations: &infrav1alpha1.NodeLocations{},
		},
	}
	infraCluster.Status.InClusterIPPoolRef = []corev1.LocalObjectReference{{Name: ipam.InClusterPoolFormat(infraCluster, infrav1alpha1.IPV4Format)}}

	infraMachine := &infrav1alpha1.ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
			Finalizers: []string{
				infrav1alpha1.MachineFinalizer,
			},
		},
		Spec: infrav1alpha1.ProxmoxMachineSpec{
			VirtualMachineCloneSpec: infrav1alpha1.VirtualMachineCloneSpec{
				SourceNode: "node1",
				TemplateID: ptr.To[int32](123),
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1.AddToScheme(scheme))
	require.NoError(t, ipamv1.AddToScheme(scheme))
	require.NoError(t, ipamicv1.AddToScheme(scheme))
	require.NoError(t, infrav1alpha1.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, machine, infraCluster, infraMachine).
		WithStatusSubresource(&infrav1alpha1.ProxmoxCluster{}, &infrav1alpha1.ProxmoxMachine{}).
		Build()

	ipamHelper := ipam.NewHelper(kubeClient, infraCluster)
	logger := logr.Discard()

	require.NoError(t, ipamHelper.CreateOrUpdateInClusterIPPool(context.Background()))

	mockClient := proxmoxtest.NewMockClient(t)

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
	suffix := infrav1alpha1.DefaultSuffix
	ip := netip.MustParseAddr(addr)
	if ip.Is6() {
		suffix += "6"
	}

	return suffix
}

func createIPAddressResource(t *testing.T, c client.Client, name, namespace, ip string, prefix int) {
	obj := &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ipamv1.IPAddressSpec{
			Address: ip,
			Prefix:  prefix,
			Gateway: netip.MustParsePrefix(fmt.Sprintf("%s/%d", ip, prefix)).Addr().Next().String(),
		},
	}
	require.NoError(t, c.Create(context.Background(), obj))
}

func createIP4AddressResource(t *testing.T, c client.Client, machineScope *scope.MachineScope, device, ip string) {
	require.Truef(t, netip.MustParseAddr(ip).Is4(), "%s is not a valid ipv4 address", ip)
	name := formatIPAddressName(machineScope.Name(), device)
	name = fmt.Sprintf("%s-%s", name, getIPSuffix(ip))

	createIPAddressResource(t, c, name, machineScope.Namespace(), ip, 24)
}

func createIP6AddressResource(t *testing.T, c client.Client, machineScope *scope.MachineScope, device, ip string) {
	require.Truef(t, netip.MustParseAddr(ip).Is6(), "%s is not a valid ipv6 address", ip)
	name := formatIPAddressName(machineScope.Name(), device)
	name = fmt.Sprintf("%s-%s", name, getIPSuffix(ip))

	createIPAddressResource(t, c, name, machineScope.Namespace(), ip, 64)
}

func createIPPools(t *testing.T, c client.Client, machineScope *scope.MachineScope) {
	for _, dev := range machineScope.ProxmoxMachine.Spec.Network.AdditionalDevices {
		poolRef := dev.IPv4PoolRef
		if poolRef == nil {
			poolRef = dev.IPv6PoolRef
		}

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

func createBootstrapSecret(t *testing.T, c client.Client, machineScope *scope.MachineScope) {
	machineScope.Machine.Spec.Bootstrap.DataSecretName = ptr.To(machineScope.Name())
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machineScope.Name(),
			Namespace: machineScope.Namespace(),
		},
		Data: map[string][]byte{
			"value": []byte("data"),
		},
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
