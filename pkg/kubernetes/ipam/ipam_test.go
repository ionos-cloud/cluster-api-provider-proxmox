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

package ipam

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	. "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/consts"
)

type IPAMTestSuite struct {
	suite.Suite
	*require.Assertions
	ctx         context.Context
	cluster     *infrav1.ProxmoxCluster
	capiCluster *clusterv1.Cluster
	cl          client.Client
	helper      *Helper
}

const otherMachineName = "other-machine"

func TestIPAMTestSuite(t *testing.T) {
	suite.Run(t, new(IPAMTestSuite))
}

func (s *IPAMTestSuite) SetupTest() {
	s.cluster = getCluster()
	s.capiCluster = &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       clusterv1.ClusterKind,
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       "test-cluster",
				Kind:       clusterv1.ClusterKind,
			}},
		},
		Spec: clusterv1.ClusterSpec{},
	}

	s.Assertions = require.New(s.T())
	scheme := scheme.Scheme

	s.NoError(clusterv1.AddToScheme(scheme))
	s.NoError(infrav1.AddToScheme(scheme))
	s.NoError(ipamicv1.AddToScheme(scheme))
	s.NoError(ipamv1.AddToScheme(scheme))

	fakeCl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(s.cluster).
		WithObjects(s.capiCluster).
		WithIndex(&ipamv1.IPAddress{}, IPAddressPoolRefNameField, IPAddressByPoolRefName).
		Build()

	s.cl = fakeCl
	s.ctx = context.Background()

	s.helper = NewHelper(s.cl, s.cluster)
}

func (s *IPAMTestSuite) Test_CreateOrUpdateInClusterIPPool() {
	ipamConfig := s.cluster.Spec.IPv4Config

	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var pool ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &pool))

	s.Len(pool.Spec.Addresses, 1)
	s.ElementsMatch(ipamConfig.Addresses, pool.Spec.Addresses)
	s.Equal(ipamConfig.Gateway, pool.Spec.Gateway)
	s.Equal(pool.Spec.Prefix, 24)

	s.cluster.Spec.IPv4Config.Gateway = "10.11.0.0"
	s.cluster.Spec.IPv4Config.Metric = ptr.To(int32(123))

	ipamConfig = s.cluster.Spec.IPv4Config

	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &pool))

	s.Equal(ipamConfig.Gateway, pool.Spec.Gateway)
	s.Equal(pool.ObjectMeta.Annotations["metric"], fmt.Sprint(*ipamConfig.Metric))

	// test deletion
	s.cluster.Spec.IPv4Config.Metric = nil
	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &pool))
	metric, exists := pool.ObjectMeta.Annotations[infrav1.ProxmoxGatewayMetricAnnotation]
	s.Equal(exists, true)
	s.Equal("", metric)

	// ipv6
	s.cluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
		Metric:    ptr.To(int32(123)),
	}

	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var poolV6 ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v6-icip",
	}, &poolV6))

	s.Len(poolV6.Spec.Addresses, 1)
	s.Equal(poolV6.ObjectMeta.Annotations["metric"], "123")
}

func (s *IPAMTestSuite) Test_GetDefaultInClusterIPPool() {
	notFound, err := s.helper.GetDefaultInClusterIPPool(s.ctx, infrav1.IPv4Format)
	s.Nil(notFound)
	s.Error(err)
	s.True(apierrors.IsNotFound(err))

	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var pool ipamicv1.InClusterIPPool

	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &pool))

	found, err := s.helper.GetDefaultInClusterIPPool(s.ctx, infrav1.IPv4Format)
	s.NoError(err)
	s.Equal(&pool, found)

	// ipv6
	s.cluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}

	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var poolV6 ipamicv1.InClusterIPPool

	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v6-icip",
	}, &poolV6))

	foundV6, err := s.helper.GetDefaultInClusterIPPool(s.ctx, infrav1.IPv6Format)
	s.NoError(err)
	s.Equal(&poolV6, foundV6)
}

func (s *IPAMTestSuite) Test_GetInClusterIPPool() {
	notFound, err := s.helper.GetInClusterIPPool(s.ctx, corev1.TypedLocalObjectReference{
		Name:     "simple-pool",
		APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind:     InClusterIPPool,
	})
	s.Nil(notFound)
	s.Error(err)
	s.True(apierrors.IsNotFound(err))

	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var pool ipamicv1.InClusterIPPool

	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &pool))

	found, err := s.helper.GetInClusterIPPool(s.ctx, corev1.TypedLocalObjectReference{
		APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Name:     "test-cluster-v4-icip",
		Kind:     InClusterIPPool})
	s.NoError(err)
	s.Equal(&pool, found)
}

func (s *IPAMTestSuite) Test_GetGlobalInClusterIPPool() {
	notFound, err := s.helper.GetGlobalInClusterIPPool(s.ctx, corev1.TypedLocalObjectReference{
		Name:     "simple-global-pool",
		APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind:     GlobalInClusterIPPool})
	s.Nil(notFound)
	s.Error(err)
	s.True(apierrors.IsNotFound(err))

	s.NoError(s.helper.ctrlClient.Create(s.ctx, &ipamicv1.GlobalInClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-global-cluster-icip",
		},
		Spec: ipamicv1.InClusterIPPoolSpec{
			Addresses: []string{"10.10.10.1-10.10.10.100"},
			Prefix:    24,
			Gateway:   "10.10.10.254",
		},
	}))

	var pool ipamicv1.GlobalInClusterIPPool

	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Name: "test-global-cluster-icip",
	}, &pool))

	found, err := s.helper.GetGlobalInClusterIPPool(s.ctx, corev1.TypedLocalObjectReference{
		Name:     "test-global-cluster-icip",
		APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
		Kind:     GlobalInClusterIPPool})

	s.NoError(err)
	s.Equal(&pool, found)
}

func (s *IPAMTestSuite) Test_GetIPPoolAnnotations() {
	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var pool ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &pool))

	ipClaimDef := IPClaimDef{
		Device: infrav1.DefaultNetworkDevice,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v4-icip",
			APIGroup: GetIPAMInClusterAPIGroup(),
			Kind:     GetInClusterIPPoolKind(),
		},
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation: "0",
		},
	}

	err := s.helper.CreateIPAddressClaim(s.ctx, getCluster(), ipClaimDef)
	s.NoError(err)

	// create a dummy IPAddress.
	err = s.cl.Create(s.ctx, s.dummyIPAddress(getCluster(), pool.GetName()))
	s.NoError(err)

	ip, err := s.helper.GetIPAddress(s.ctx, client.ObjectKeyFromObject(s.cluster))
	s.NoError(err)
	s.NotNil(ip)
	s.NotEmpty(ip.Spec.Address)
	s.Equal(ip.Spec.Address, "10.10.10.11")

	annotations, err := s.helper.GetIPPoolAnnotations(s.ctx, ip)
	s.NotNil(annotations)
	s.Nil(err)

	s.NoError(s.helper.ctrlClient.Create(s.ctx, &ipamicv1.GlobalInClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ippool-annotations",
			Annotations: map[string]string{
				"metric": "100",
			},
		},
		Spec: ipamicv1.InClusterIPPoolSpec{
			Addresses: []string{"10.10.11.1-10.10.11.100"},
			Prefix:    24,
			Gateway:   "10.10.11.254",
		},
	}))

	var globalPool ipamicv1.GlobalInClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Name: "test-ippool-annotations",
	}, &globalPool))

	ipClaimDef = IPClaimDef{
		Device: infrav1.DefaultNetworkDevice,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-ippool-annotations",
			APIGroup: GetIPAMInClusterAPIGroup(),
			Kind:     GetGlobalInClusterIPPoolKind(),
		},
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation: "0",
		},
	}

	err = s.helper.CreateIPAddressClaim(s.ctx, getCluster(), ipClaimDef)
	s.NoError(err)

	gvk, err := apiutil.GVKForObject(&globalPool, s.cl.Scheme())
	if err != nil {
		panic(err)
	}

	ip = &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getCluster().GetName(),
			Namespace: getCluster().GetNamespace(),
		},
		Spec: ipamv1.IPAddressSpec{
			ClaimRef: ipamv1.IPAddressClaimReference{
				Name: getCluster().GetName(),
			},
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: gvk.Group,
				Kind:     gvk.Kind,
				Name:     "test-ippool-annotations",
			},
			Address: "10.10.11.11",
			Prefix:  ptr.To[int32](24),
			Gateway: "10.10.11.254",
		},
	}

	annotations, err = s.helper.GetIPPoolAnnotations(s.ctx, ip)
	s.NotNil(annotations)
	s.Nil(err)

	s.Equal(annotations["metric"], "100")
}

func (s *IPAMTestSuite) Test_CreateIPAddressClaimv2() {
	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	// default device
	var pool ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &pool))

	device := infrav1.DefaultNetworkDevice

	ipClaimDef := IPClaimDef{
		Device: device,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v4-icip",
			APIGroup: GetIPAMInClusterAPIGroup(),
			Kind:     GetInClusterIPPoolKind(),
		},
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation: "0",
		},
	}

	err := s.helper.CreateIPAddressClaim(s.ctx, getCluster(), ipClaimDef)
	s.NoError(err)

	// Ensure cluster label is set.
	var claim ipamv1.IPAddressClaim
	name := IPAddressFormat(getCluster().GetName(), device, 0, infrav1.DefaultSuffix)
	nn := types.NamespacedName{Name: name, Namespace: getCluster().GetNamespace()}
	err = s.cl.Get(s.ctx, nn, &claim)
	s.NoError(err)
	s.Contains(claim.ObjectMeta.Labels, clusterv1.ClusterNameLabel)
	s.Equal(getCluster().GetName(), claim.ObjectMeta.Labels[clusterv1.ClusterNameLabel])

	// additional device with InClusterIPPool
	s.NoError(s.helper.ctrlClient.Create(s.ctx, &ipamicv1.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "test-additional-cluster-icip",
		},
		Spec: ipamicv1.InClusterIPPoolSpec{
			Addresses: []string{"10.10.10.1-10.10.10.100"},
			Prefix:    24,
			Gateway:   "10.10.10.254",
		},
	}))

	var additionalPool ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-additional-cluster-icip",
	}, &additionalPool))

	ipClaimDef = IPClaimDef{
		Device: "net1",
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v4-icip",
			APIGroup: GetIPAMInClusterAPIGroup(),
			Kind:     GetInClusterIPPoolKind(),
		},
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation: "0",
		},
	}

	err = s.helper.CreateIPAddressClaim(s.ctx, getCluster(), ipClaimDef)
	s.NoError(err)

	// additional device with GlobalInClusterIPPool
	s.NoError(s.helper.ctrlClient.Create(s.ctx, &ipamicv1.GlobalInClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-global-cluster-icip",
		},
		Spec: ipamicv1.InClusterIPPoolSpec{
			Addresses: []string{"10.10.10.1-10.10.10.100"},
			Prefix:    24,
			Gateway:   "10.10.10.254",
		},
	}))

	var globalPool ipamicv1.GlobalInClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Name: "test-global-cluster-icip",
	}, &globalPool))

	ipClaimDef = IPClaimDef{
		Device: "net2",
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-global-cluster-icip",
			APIGroup: GetIPAMInClusterAPIGroup(),
			Kind:     GetGlobalInClusterIPPoolKind(),
		},
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation: "0",
		},
	}

	err = s.helper.CreateIPAddressClaim(s.ctx, getCluster(), ipClaimDef)
	s.NoError(err)

	// IPv6.
	s.cluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}
	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var poolV6 ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v6-icip",
	}, &poolV6))

	ipClaimDef = IPClaimDef{
		Device: device,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v6-icip",
			APIGroup: GetIPAMInClusterAPIGroup(),
			Kind:     GetInClusterIPPoolKind(),
		},
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation: "0",
		},
	}

	err = s.helper.CreateIPAddressClaim(s.ctx, getCluster(), ipClaimDef)
	s.NoError(err)
}

func (s *IPAMTestSuite) Test_GetIPAddress() {
	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var pool ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &pool))

	ipClaimDef := IPClaimDef{
		Device: infrav1.DefaultNetworkDevice,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v4-icip",
			APIGroup: GetIPAMInClusterAPIGroup(),
			Kind:     GetInClusterIPPoolKind(),
		},
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation: "0",
		},
	}

	err := s.helper.CreateIPAddressClaim(s.ctx, getCluster(), ipClaimDef)
	s.NoError(err)

	// create a dummy IPAddress.
	err = s.cl.Create(s.ctx, s.dummyIPAddress(getCluster(), pool.GetName()))
	s.NoError(err)

	ip, err := s.helper.GetIPAddress(s.ctx, client.ObjectKeyFromObject(s.cluster))
	s.NoError(err)
	s.NotNil(ip)
	s.NotEmpty(ip.Spec.Address)
	s.Equal(ip.Spec.Address, "10.10.10.11")
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimMissing() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimMissing, result.Status)
	s.Equal(IPAddressFormat(machine.Name, infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix), result.ClaimName)
	s.Nil(result.Claim)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimInvalidOffset() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "not-an-int", "test-cluster-v4-icip")

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.Error(err)
	s.Empty(result)
}

func (s *IPAMTestSuite) Test_IPClaimNameDefaultsMissingOffset() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef("net1", "2", "test-cluster-v4-icip")
	delete(ipClaimDef.Annotations, infrav1.ProxmoxPoolOffsetAnnotation)

	name, err := ipClaimName(machine, ipClaimDef)

	s.NoError(err)
	s.Equal(IPAddressFormat(machine.Name, "net1", 0, infrav1.DefaultSuffix), name)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimPending() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "")
	s.NoError(s.cl.Create(s.ctx, claim))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimPending, result.Status)
	s.Equal(claim.Name, result.ClaimName)
	s.Equal(claim.Name, result.Claim.Name)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimResolved() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "allocated-address")
	address := s.testIPAddress(claim.Namespace, "allocated-address", "test-cluster-v4-icip")
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimResolved, result.Status)
	s.NotNil(result.Address)
	s.Equal(address.Name, result.Address.Name)
	s.Equal("10.10.10.11", result.Address.Spec.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimResolvedMergesClaimAnnotations() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "2", "test-cluster-v4-icip")
	ipClaimDef.Annotations[infrav1.ProxmoxDefaultGatewayAnnotation] = "true"
	claim := s.testIPAddressClaim(machine, ipClaimDef, "allocated-address")
	address := s.testIPAddress(claim.Namespace, "allocated-address", "test-cluster-v4-icip")
	address.Annotations = map[string]string{"address-annotation": "preserved"}
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimResolved, result.Status)
	s.Equal("preserved", result.Address.Annotations["address-annotation"])
	s.Equal("2", result.Address.Annotations[infrav1.ProxmoxPoolOffsetAnnotation])
	s.Equal("true", result.Address.Annotations[infrav1.ProxmoxDefaultGatewayAnnotation])
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimConflictOwnerMismatch() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "")
	claim.OwnerReferences[0].UID = types.UID("other-uid")
	claim.OwnerReferences[0].Name = otherMachineName
	s.NoError(s.cl.Create(s.ctx, claim))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimConflict, result.Status)
	s.Equal(ConflictOwnerMismatch, result.ConflictReason)
	s.Equal(claim.Name, result.Claim.Name)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimConflictPoolMismatch() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "")
	claim.Spec.PoolRef.Name = "other-pool"
	s.NoError(s.cl.Create(s.ctx, claim))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimConflict, result.Status)
	s.Equal(ConflictPoolMismatch, result.ConflictReason)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimConflictAddressMissing() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "missing-address")
	s.NoError(s.cl.Create(s.ctx, claim))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimConflict, result.Status)
	s.Equal(ConflictAddressMissing, result.ConflictReason)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimConflictAddressPoolRef() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "allocated-address")
	address := s.testIPAddress(claim.Namespace, "allocated-address", "other-pool")
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimConflict, result.Status)
	s.Equal(ConflictAddressPoolRef, result.ConflictReason)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimDirectOwnerReferenceFallback() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "allocated-address")
	claim.OwnerReferences[0].APIVersion = infrav1.GroupVersion.Group
	address := s.testIPAddress(claim.Namespace, "allocated-address", "test-cluster-v4-icip")
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimResolved, result.Status)
	s.NotNil(result.Address)
	s.Equal(address.Name, result.Address.Name)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimMissingWithOrphanedDeterministicIPAddress() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	orphanName := IPAddressFormat(machine.Name, infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix)
	s.NoError(s.cl.Create(s.ctx, s.testIPAddress(machine.Namespace, orphanName, "test-cluster-v4-icip")))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimMissing, result.Status)
	s.Equal(orphanName, result.OrphanedAddressName)
	s.Nil(result.Claim)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_GetIPAddressByPoolFiltersByPoolRefAndSorts() {
	poolRef := corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     GetInClusterIPPoolKind(),
	}

	matchingB := s.testIPAddress("test", "matching-b", poolRef.Name)
	matchingA := s.testIPAddress("test", "matching-a", poolRef.Name)
	wrongKind := s.testIPAddress("test", "wrong-kind", poolRef.Name)
	wrongKind.Spec.PoolRef.Kind = GetGlobalInClusterIPPoolKind()
	wrongGroup := s.testIPAddress("test", "wrong-group", poolRef.Name)
	wrongGroup.Spec.PoolRef.APIGroup = "other.ipam.example.com"
	otherPool := s.testIPAddress("test", "other-pool", "other-pool")
	s.NoError(s.cl.Create(s.ctx, matchingB))
	s.NoError(s.cl.Create(s.ctx, matchingA))
	s.NoError(s.cl.Create(s.ctx, wrongKind))
	s.NoError(s.cl.Create(s.ctx, wrongGroup))
	s.NoError(s.cl.Create(s.ctx, otherPool))

	addresses, err := s.helper.GetIPAddressByPool(s.ctx, poolRef)

	s.NoError(err)
	s.Len(addresses, 2)
	s.Equal("matching-a", addresses[0].Name)
	s.Equal("matching-b", addresses[1].Name)
}

func (s *IPAMTestSuite) Test_GetIPAddressV2ReturnsOwnedAddressesAndMergesClaimAnnotations() {
	machine := s.testMachine()
	poolRef := corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     GetInClusterIPPoolKind(),
	}

	ownedClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", poolRef.Name)
	ownedClaim := s.testIPAddressClaim(machine, ownedClaimDef, "")
	ownedAddress := s.testIPAddress(machine.Namespace, ownedClaim.Name, poolRef.Name)
	ownedAddress.Annotations = nil
	s.NoError(s.cl.Create(s.ctx, ownedClaim))
	s.NoError(s.cl.Create(s.ctx, ownedAddress))

	otherClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "1", poolRef.Name)
	otherClaim := s.testIPAddressClaim(machine, otherClaimDef, "")
	otherClaim.OwnerReferences[0].UID = types.UID("other-uid")
	otherClaim.OwnerReferences[0].Name = otherMachineName
	otherAddress := s.testIPAddress(machine.Namespace, otherClaim.Name, poolRef.Name)
	s.NoError(s.cl.Create(s.ctx, otherClaim))
	s.NoError(s.cl.Create(s.ctx, otherAddress))

	addresses, err := s.helper.GetIPAddressV2(s.ctx, poolRef, machine)

	s.NoError(err)
	s.Len(addresses, 1)
	s.Equal(ownedAddress.Name, addresses[0].Name)
	s.Equal("0", addresses[0].Annotations[infrav1.ProxmoxPoolOffsetAnnotation])
}

func (s *IPAMTestSuite) Test_HasDirectOwnerReferenceRequiresExactMachineIdentity() {
	machine := s.testMachine()
	ownerRef := metav1.OwnerReference{
		APIVersion: infrav1.GroupVersion.String(),
		Kind:       infrav1.ProxmoxMachineKind,
		Name:       machine.Name,
		UID:        machine.UID,
	}

	s.True(hasDirectOwnerReference([]metav1.OwnerReference{ownerRef}, machine))

	groupOnly := ownerRef
	groupOnly.APIVersion = infrav1.GroupVersion.Group
	s.True(hasDirectOwnerReference([]metav1.OwnerReference{groupOnly}, machine))

	uidMismatch := ownerRef
	uidMismatch.UID = types.UID("other-uid")
	s.False(hasDirectOwnerReference([]metav1.OwnerReference{uidMismatch}, machine))

	nameMismatch := ownerRef
	nameMismatch.Name = otherMachineName
	s.False(hasDirectOwnerReference([]metav1.OwnerReference{nameMismatch}, machine))

	kindMismatch := ownerRef
	kindMismatch.Kind = "OtherMachine"
	s.False(hasDirectOwnerReference([]metav1.OwnerReference{kindMismatch}, machine))

	apiGroupMismatch := ownerRef
	apiGroupMismatch.APIVersion = "other.infrastructure.cluster.x-k8s.io/v1alpha2"
	s.False(hasDirectOwnerReference([]metav1.OwnerReference{apiGroupMismatch}, machine))
}

func (s *IPAMTestSuite) Test_MatchesPoolRefIgnoresIPAddressTypeMeta() {
	ip := ipamv1.IPAddress{
		TypeMeta: metav1.TypeMeta{Kind: "IPAddress", APIVersion: "unrelated.example.com/v1"},
		Spec: ipamv1.IPAddressSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: "ipam.cluster.x-k8s.io",
				Kind:     GetInClusterIPPoolKind(),
				Name:     "test-cluster-v4-icip",
			},
		},
	}

	s.True(matchesPoolRef(ip, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: GetIPAMInClusterAPIGroup(),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesPoolRef(ip, corev1.TypedLocalObjectReference{
		Name:     "other-pool",
		APIGroup: GetIPAMInClusterAPIGroup(),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesPoolRef(ip, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: ptr.To("other.ipam.example.com"),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesPoolRef(ip, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: GetIPAMInClusterAPIGroup(),
		Kind:     GetGlobalInClusterIPPoolKind(),
	}))
}

func (s *IPAMTestSuite) Test_MatchesClaimPoolRefComparesNameGroupAndKind() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "")

	s.True(matchesClaimPoolRef(*claim, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesClaimPoolRef(*claim, corev1.TypedLocalObjectReference{
		Name:     "other-pool",
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesClaimPoolRef(*claim, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: ptr.To("other.ipam.example.com"),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesClaimPoolRef(*claim, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     GetGlobalInClusterIPPoolKind(),
	}))
}

func getCluster() *infrav1.ProxmoxCluster {
	return &infrav1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       infrav1.ProxmoxClusterKind,
			APIVersion: infrav1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       "test-cluster",
				Kind:       clusterv1.ClusterKind,
			}},
		},
		Spec: infrav1.ProxmoxClusterSpec{
			IPv4Config: &infrav1.IPConfigSpec{
				Addresses: []string{"10.10.0.1/24"},
				Gateway:   "10.0.0.0",
				Prefix:    24,
			},
		},
	}
}

func (s *IPAMTestSuite) testMachine() *infrav1.ProxmoxMachine {
	return &infrav1.ProxmoxMachine{
		TypeMeta: metav1.TypeMeta{
			Kind:       infrav1.ProxmoxMachineKind,
			APIVersion: infrav1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "test",
			UID:       types.UID("test-machine-uid"),
		},
	}
}

func (s *IPAMTestSuite) testIPClaimDef(device infrav1.NetName, offset, poolName string) IPClaimDef {
	return IPClaimDef{
		Device: device,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     poolName,
			APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
			Kind:     GetInClusterIPPoolKind(),
		},
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation: offset,
		},
	}
}

func (s *IPAMTestSuite) testIPAddressClaim(machine *infrav1.ProxmoxMachine, ipClaimDef IPClaimDef, addressName string) *ipamv1.IPAddressClaim {
	claimName, err := ipClaimName(machine, ipClaimDef)
	s.NoError(err)

	return &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        claimName,
			Namespace:   machine.Namespace,
			Annotations: ipClaimDef.Annotations,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       infrav1.ProxmoxMachineKind,
				Name:       machine.Name,
				UID:        machine.UID,
			}},
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: ipamicv1.GroupVersion.Group,
				Kind:     ipClaimDef.PoolRef.Kind,
				Name:     ipClaimDef.PoolRef.Name,
			},
		},
		Status: ipamv1.IPAddressClaimStatus{
			AddressRef: ipamv1.IPAddressReference{Name: addressName},
		},
	}
}

func (s *IPAMTestSuite) testIPAddress(namespace, name, poolName string) *ipamv1.IPAddress {
	return &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ipamv1.IPAddressSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: ipamicv1.GroupVersion.Group,
				Kind:     GetInClusterIPPoolKind(),
				Name:     poolName,
			},
			Address: "10.10.10.11",
			Prefix:  ptr.To[int32](24),
			Gateway: "10.10.10.1",
		},
	}
}

func (s *IPAMTestSuite) dummyIPAddress(owner client.Object, poolName string) *ipamv1.IPAddress {
	gvk, err := apiutil.GVKForObject(new(ipamicv1.InClusterIPPool), s.cl.Scheme())
	if err != nil {
		panic(err)
	}
	return &ipamv1.IPAddress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IPAddress",
			APIVersion: "ipam.cluster.x-k8s.io/v1beta2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.GetName(),
			Namespace: owner.GetNamespace(),
		},
		Spec: ipamv1.IPAddressSpec{
			ClaimRef: ipamv1.IPAddressClaimReference{
				Name: owner.GetName(),
			},
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: gvk.Group,
				Kind:     gvk.Kind,
				Name:     poolName,
			},
			Address: "10.10.10.11",
			Prefix:  ptr.To[int32](24),
			Gateway: "10.10.10.1",
		},
	}
}
