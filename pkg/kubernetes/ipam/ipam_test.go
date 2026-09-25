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
	"maps"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

const (
	otherClusterName = "other-cluster"
	otherPoolName    = "other-pool"
	otherClaimName   = "other-claim"
)

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
	s.cluster.Spec.IPv4Config.Metric = new(int32(123))

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
		Metric:    new(int32(123)),
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

// Test_CreateOrUpdateInClusterIPPool_AddressReuseGracePeriod verifies that
// CreateOrUpdateInClusterIPPool sets AddressReuseGracePeriodSeconds on both
// IPv4 and IPv6 pools to prevent immediate IP reuse after node deletion.
func (s *IPAMTestSuite) Test_CreateOrUpdateInClusterIPPool_AddressReuseGracePeriod() {
	s.cluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}

	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))

	var poolV4 ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v4-icip",
	}, &poolV4))
	s.NotNil(poolV4.Spec.AddressReuseGracePeriodSeconds)
	s.EqualValues(60, *poolV4.Spec.AddressReuseGracePeriodSeconds)

	var poolV6 ipamicv1.InClusterIPPool
	s.NoError(s.cl.Get(s.ctx, types.NamespacedName{
		Namespace: "test",
		Name:      "test-cluster-v6-icip",
	}, &poolV6))
	s.NotNil(poolV6.Spec.AddressReuseGracePeriodSeconds)
	s.EqualValues(60, *poolV6.Spec.AddressReuseGracePeriodSeconds)
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
		APIGroup: new("ipam.cluster.x-k8s.io"),
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
		APIGroup: new("ipam.cluster.x-k8s.io"),
		Name:     "test-cluster-v4-icip",
		Kind:     InClusterIPPool})
	s.NoError(err)
	s.Equal(&pool, found)
}

func (s *IPAMTestSuite) Test_GetGlobalInClusterIPPool() {
	notFound, err := s.helper.GetGlobalInClusterIPPool(s.ctx, corev1.TypedLocalObjectReference{
		Name:     "simple-global-pool",
		APIGroup: new("ipam.cluster.x-k8s.io"),
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
		APIGroup: new("ipam.cluster.x-k8s.io"),
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
			Prefix:  new(int32(24)),
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

func (s *IPAMTestSuite) Test_CreateIPAddressClaimDoesNotMutateConcurrentExistingClaim() {
	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	existing := s.testIPAddressClaim(machine, ipClaimDef, "")
	existing.OwnerReferences[0].Name = "other-machine"
	existing.OwnerReferences[0].UID = types.UID("other-machine-uid")
	existing.Annotations["preserved"] = "value"
	s.NoError(s.cl.Create(s.ctx, existing))
	before := existing.DeepCopy()

	created, err := s.helper.CreateIPAddressClaimIfMissing(s.ctx, machine, ipClaimDef)
	s.NoError(err)
	s.False(created)

	var actual ipamv1.IPAddressClaim
	s.NoError(s.cl.Get(s.ctx, client.ObjectKeyFromObject(existing), &actual))
	s.Equal(before, &actual)
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

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimOwnerlessPendingIsAdoptable() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "")
	claim.OwnerReferences = nil
	s.NoError(s.cl.Create(s.ctx, claim))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimAdoptable, result.Status)
	s.Equal(claim.Name, result.ClaimName)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimUsesCurrentClusterForRecoveryProvenance() {
	machine := s.testMachine()
	machine.Labels[clusterv1.ClusterNameLabel] = "stale-cluster"
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "")
	claim.OwnerReferences = nil
	claim.Labels[clusterv1.ClusterNameLabel] = "stale-cluster"
	s.NoError(s.cl.Create(s.ctx, claim))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimConflict, result.Status)
	s.Equal(ConflictClaimCluster, result.ConflictReason)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimResolved() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "allocated-address")
	address := s.testIPAddress(claim.Namespace, "allocated-address", "test-cluster-v4-icip", claim.Name)
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimResolved, result.Status)
	s.NotNil(result.Address)
	s.Equal(address.Name, result.Address.Name)
	s.Equal("192.0.2.1", result.Address.Spec.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimResolvedMergesClaimAnnotations() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "2", "test-cluster-v4-icip")
	ipClaimDef.Annotations[infrav1.ProxmoxDefaultGatewayAnnotation] = "true"
	claim := s.testIPAddressClaim(machine, ipClaimDef, "allocated-address")
	address := s.testIPAddress(claim.Namespace, "allocated-address", "test-cluster-v4-icip", claim.Name)
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
	claim.Spec.PoolRef.Name = otherPoolName
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
	address := s.testIPAddress(claim.Namespace, "allocated-address", otherPoolName, claim.Name)
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimConflict, result.Status)
	s.Equal(ConflictAddressPoolRef, result.ConflictReason)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimConflictAddressClaimRef() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "allocated-address")
	address := s.testIPAddress(claim.Namespace, "allocated-address", "test-cluster-v4-icip", otherClaimName)
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimConflict, result.Status)
	s.Equal(ConflictAddressClaimRef, result.ConflictReason)
	s.Equal(address.Name, result.ConflictingAddress.Name)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimDirectOwnerReferenceFallback() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "allocated-address")
	claim.OwnerReferences[0].APIVersion = infrav1.GroupVersion.Group
	address := s.testIPAddress(claim.Namespace, "allocated-address", "test-cluster-v4-icip", claim.Name)
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
	s.Equal(orphanName, result.OrphanedAddress.Name)
	s.Nil(result.Claim)
	s.Nil(result.Address)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimOwnerlessValidIsAdoptable() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "")
	claim.Status.AddressRef.Name = claim.Name
	claim.OwnerReferences = nil
	claim.UID = types.UID("restored-claim-uid")
	address := s.testIPAddress(claim.Namespace, claim.Name, "test-cluster-v4-icip", claim.Name)
	address.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: ipamv1.GroupVersion.String(),
		Kind:       "IPAddressClaim",
		Name:       claim.Name,
		UID:        claim.UID,
		Controller: new(true),
	}}
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimAdoptable, result.Status)
	s.Equal(claim.Name, result.Claim.Name)
	s.Equal(address.Name, result.Address.Name)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimOwnerlessRejectsFailedProvenanceChecks() {
	tests := []struct {
		name   string
		reason IPAddressClaimConflictReason
		mutate func(*ipamv1.IPAddressClaim, *ipamv1.IPAddress)
	}{
		{
			name:   "claim cluster label",
			reason: ConflictClaimCluster,
			mutate: func(claim *ipamv1.IPAddressClaim, _ *ipamv1.IPAddress) {
				claim.Labels[clusterv1.ClusterNameLabel] = otherClusterName
			},
		},
		{
			name:   "claim annotations",
			reason: ConflictClaimAnnotations,
			mutate: func(claim *ipamv1.IPAddressClaim, _ *ipamv1.IPAddress) {
				claim.Annotations[infrav1.ProxmoxPoolOffsetAnnotation] = "99"
			},
		},
		{
			name:   "unexpected default gateway annotation",
			reason: ConflictClaimAnnotations,
			mutate: func(claim *ipamv1.IPAddressClaim, _ *ipamv1.IPAddress) {
				claim.Annotations[infrav1.ProxmoxDefaultGatewayAnnotation] = "true"
			},
		},
		{
			name:   "claim address reference",
			reason: ConflictAddressRef,
			mutate: func(claim *ipamv1.IPAddressClaim, _ *ipamv1.IPAddress) {
				claim.Status.AddressRef.Name = "non-deterministic-address"
			},
		},
		{
			name:   "address claim reference",
			reason: ConflictAddressClaimRef,
			mutate: func(_ *ipamv1.IPAddressClaim, address *ipamv1.IPAddress) {
				address.Spec.ClaimRef.Name = otherClaimName
			},
		},
		{
			name:   "address cluster label",
			reason: ConflictAddressCluster,
			mutate: func(_ *ipamv1.IPAddressClaim, address *ipamv1.IPAddress) {
				address.Labels[clusterv1.ClusterNameLabel] = otherClusterName
			},
		},
		{
			name:   "address controller owner",
			reason: ConflictAddressOwner,
			mutate: func(_ *ipamv1.IPAddressClaim, address *ipamv1.IPAddress) {
				address.OwnerReferences = []metav1.OwnerReference{{
					APIVersion: ipamv1.GroupVersion.String(),
					Kind:       "IPAddressClaim",
					Name:       otherClaimName,
					UID:        types.UID("other-claim-uid"),
					Controller: new(true),
				}}
			},
		},
	}

	for i, tt := range tests {
		s.Run(tt.name, func() {
			machine := s.testMachine()
			ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, fmt.Sprint(i), "test-cluster-v4-icip")
			claim := s.testIPAddressClaim(machine, ipClaimDef, "")
			claim.Status.AddressRef.Name = claim.Name
			claim.OwnerReferences = nil
			claim.UID = types.UID(fmt.Sprintf("claim-uid-%d", i))
			address := s.testIPAddress(claim.Namespace, claim.Name, "test-cluster-v4-icip", claim.Name)
			tt.mutate(claim, address)
			s.NoError(s.cl.Create(s.ctx, claim))
			s.NoError(s.cl.Create(s.ctx, address))

			result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

			s.NoError(err)
			s.Equal(ClaimConflict, result.Status)
			s.Equal(tt.reason, result.ConflictReason)
			s.Nil(result.Address)
		})
	}
}

func (s *IPAMTestSuite) Test_AdoptPendingClaimValidatesExistingAddress() {
	tests := []struct {
		name   string
		reason IPAddressClaimConflictReason
		mutate func(*ipamv1.IPAddress)
	}{
		{name: "valid", mutate: func(_ *ipamv1.IPAddress) {}},
		{name: "cluster", reason: ConflictAddressCluster, mutate: func(address *ipamv1.IPAddress) {
			address.Labels[clusterv1.ClusterNameLabel] = otherClusterName
		}},
		{name: "pool", reason: ConflictAddressPoolRef, mutate: func(address *ipamv1.IPAddress) {
			address.Spec.PoolRef.Name = otherPoolName
		}},
		{name: "claimRef", reason: ConflictAddressClaimRef, mutate: func(address *ipamv1.IPAddress) {
			address.Spec.ClaimRef.Name = otherClaimName
		}},
		{name: "owner", reason: ConflictAddressOwner, mutate: func(address *ipamv1.IPAddress) {
			address.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: ipamv1.GroupVersion.String(), Kind: "IPAddressClaim",
				Name: address.Name, UID: types.UID("stale-claim-uid"), Controller: new(true),
			}}
		}},
	}
	for i, tt := range tests {
		s.Run(tt.name, func() {
			machine := s.testMachine()
			def := s.testIPClaimDef(infrav1.DefaultNetworkDevice, fmt.Sprint(i), "test-cluster-v4-icip")
			claim := s.testIPAddressClaim(machine, def, "")
			claim.OwnerReferences = nil
			address := s.testIPAddress(claim.Namespace, claim.Name, def.PoolRef.Name)
			tt.mutate(address)
			s.NoError(s.cl.Create(s.ctx, claim))
			s.NoError(s.cl.Create(s.ctx, address))
			claimBefore := claim.DeepCopy()
			addressBefore := address.DeepCopy()

			result, adopted, err := s.helper.AdoptIPAddressClaim(s.ctx, machine, def)
			s.NoError(err)
			s.NoError(s.cl.Get(s.ctx, client.ObjectKeyFromObject(claim), claim))
			s.NoError(s.cl.Get(s.ctx, client.ObjectKeyFromObject(address), address))
			s.Equal(addressBefore, address)
			s.Nil(result.Address, "an address is not resolved until status.addressRef is populated")
			if tt.reason == "" {
				s.True(adopted)
				s.Equal(ClaimPending, result.Status)
				s.Empty(claim.Status.AddressRef.Name)
			} else {
				s.False(adopted)
				s.Equal(ClaimConflict, result.Status)
				s.Equal(tt.reason, result.ConflictReason)
				s.Equal(claimBefore, claim)
			}

			// Simulate IPAM publishing its reference after CAPMOX attempted adoption.
			claim.Status.AddressRef.Name = address.Name
			s.NoError(s.cl.Update(s.ctx, claim))
			result, err = s.helper.ResolveIPAddressClaim(s.ctx, machine, def)
			s.NoError(err)
			if tt.reason == "" {
				s.Equal(ClaimResolved, result.Status)
			} else {
				s.Equal(ClaimConflict, result.Status)
				s.Equal(tt.reason, result.ConflictReason)
			}
		})
	}
}

func (s *IPAMTestSuite) Test_ResolveAndAdoptIPAddressClaimRejectsTerminatingClaims() {
	for _, ownerless := range []bool{true, false} {
		for _, pending := range []bool{false, true} {
			s.Run(fmt.Sprintf("ownerless=%t/pending=%t", ownerless, pending), func() {
				machine := s.testMachine()
				machine.Name = fmt.Sprintf("%s-%t-%t", machine.Name, ownerless, pending)
				def := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
				claim := s.testIPAddressClaim(machine, def, "")
				claim.Finalizers = []string{"test.finalizer"}
				if ownerless {
					claim.OwnerReferences = nil
				}
				if !pending {
					claim.Status.AddressRef.Name = claim.Name
				}
				address := s.testIPAddress(claim.Namespace, claim.Name, def.PoolRef.Name)
				s.NoError(s.cl.Create(s.ctx, claim))
				s.NoError(s.cl.Create(s.ctx, address))
				s.NoError(s.cl.Delete(s.ctx, claim))
				s.NoError(s.cl.Get(s.ctx, client.ObjectKeyFromObject(claim), claim))
				s.False(claim.DeletionTimestamp.IsZero())
				claimBefore := claim.DeepCopy()
				addressBefore := address.DeepCopy()

				result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, def)
				s.NoError(err)
				s.Equal(ClaimConflict, result.Status)
				s.Equal(ConflictClaimDeleting, result.ConflictReason)
				s.Nil(result.Address)

				result, adopted, err := s.helper.AdoptIPAddressClaim(s.ctx, machine, def)
				s.NoError(err)
				s.False(adopted)
				s.Equal(ClaimConflict, result.Status)
				s.Nil(result.Address)
				s.NoError(s.cl.Get(s.ctx, client.ObjectKeyFromObject(claim), claim))
				s.NoError(s.cl.Get(s.ctx, client.ObjectKeyFromObject(address), address))
				s.Equal(claimBefore, claim)
				s.Equal(addressBefore, address)
			})
		}
	}
}

func (s *IPAMTestSuite) Test_AdoptIPAddressClaimOnlyChangesOwnerReferencesAndIsIdempotent() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, ipClaimDef, "")
	claim.Status.AddressRef.Name = claim.Name
	claim.OwnerReferences = nil
	claim.Finalizers = []string{"test.finalizer"}
	claim.Annotations["preserved"] = "value"
	address := s.testIPAddress(claim.Namespace, claim.Name, "test-cluster-v4-icip", claim.Name)
	s.NoError(s.cl.Create(s.ctx, claim))
	s.NoError(s.cl.Create(s.ctx, address))

	before := claim.DeepCopy()
	addressBefore := address.DeepCopy()
	result, adopted, err := s.helper.AdoptIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.True(adopted)
	s.Equal(ClaimResolved, result.Status)
	var adoptedClaim ipamv1.IPAddressClaim
	s.NoError(s.cl.Get(s.ctx, client.ObjectKeyFromObject(claim), &adoptedClaim))
	s.Equal(before.Spec, adoptedClaim.Spec)
	s.Equal(before.Status, adoptedClaim.Status)
	s.Equal(before.Labels, adoptedClaim.Labels)
	s.Equal(before.Annotations, adoptedClaim.Annotations)
	s.Equal(before.Finalizers, adoptedClaim.Finalizers)
	s.Len(adoptedClaim.OwnerReferences, 1)
	s.Equal(machine.Name, adoptedClaim.OwnerReferences[0].Name)
	s.Equal(machine.UID, adoptedClaim.OwnerReferences[0].UID)
	s.True(ptr.Deref(adoptedClaim.OwnerReferences[0].Controller, false))
	var addressAfter ipamv1.IPAddress
	s.NoError(s.cl.Get(s.ctx, client.ObjectKeyFromObject(address), &addressAfter))
	s.Equal(addressBefore, &addressAfter)

	result, adopted, err = s.helper.AdoptIPAddressClaim(s.ctx, machine, ipClaimDef)
	s.NoError(err)
	s.False(adopted)
	s.Equal(ClaimResolved, result.Status)
}

func (s *IPAMTestSuite) Test_AdoptIPAddressClaimPropagatesReadFailures() {
	tests := []struct {
		name         string
		missingClaim bool
		failObject   client.Object
	}{
		{name: "claim lookup", failObject: &ipamv1.IPAddressClaim{}},
		{name: "allocated address lookup", failObject: &ipamv1.IPAddress{}},
		{name: "claim cluster lookup", failObject: &clusterv1.Cluster{}},
		{name: "orphan address lookup", missingClaim: true, failObject: &ipamv1.IPAddress{}},
		{name: "orphan cluster lookup", missingClaim: true, failObject: &clusterv1.Cluster{}},
	}
	for i, tt := range tests {
		s.Run(tt.name, func() {
			machine := s.testMachine()
			def := s.testIPClaimDef(infrav1.DefaultNetworkDevice, fmt.Sprint(i), "test-cluster-v4-icip")
			claim := s.testIPAddressClaim(machine, def, "")
			claim.OwnerReferences = nil
			claim.Status.AddressRef.Name = claim.Name
			address := s.testIPAddress(claim.Namespace, claim.Name, def.PoolRef.Name)
			if !tt.missingClaim {
				s.NoError(s.cl.Create(s.ctx, claim))
			}
			s.NoError(s.cl.Create(s.ctx, address))
			claimBefore := claim.DeepCopy()
			addressBefore := address.DeepCopy()
			lookupErr := apierrors.NewServiceUnavailable("IPAM lookup unavailable")
			builder := fake.NewClientBuilder().WithScheme(s.cl.Scheme()).WithObjects(s.capiCluster, address).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if fmt.Sprintf("%T", obj) == fmt.Sprintf("%T", tt.failObject) {
							return lookupErr
						}
						return cl.Get(ctx, key, obj, opts...)
					},
				})
			if !tt.missingClaim {
				builder.WithObjects(claim)
			}
			cl := builder.Build()
			result, adopted, err := NewHelper(cl, s.cluster).AdoptIPAddressClaim(s.ctx, machine, def)
			s.ErrorIs(err, lookupErr)
			s.False(adopted)
			s.Nil(result.Address)
			// Read failures must not be mistaken for absent objects or successful adoption.
			claims := &ipamv1.IPAddressClaimList{}
			addresses := &ipamv1.IPAddressList{}
			s.NoError(cl.List(s.ctx, claims))
			s.NoError(cl.List(s.ctx, addresses))
			if tt.missingClaim {
				s.Empty(claims.Items)
			} else {
				s.Len(claims.Items, 1)
				s.Equal(claimBefore, &claims.Items[0])
			}
			s.Equal([]ipamv1.IPAddress{*addressBefore}, addresses.Items)
		})
	}
}

func (s *IPAMTestSuite) Test_AdoptIPAddressClaimRejectsConcurrentUpdate() {
	machine := s.testMachine()
	def := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	claim := s.testIPAddressClaim(machine, def, "")
	claim.OwnerReferences = nil
	cl := fake.NewClientBuilder().WithScheme(s.cl.Scheme()).WithObjects(s.capiCluster, claim).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				// Another reconciler changes the claim between validation and adoption.
				current := &ipamv1.IPAddressClaim{}
				s.NoError(cl.Get(ctx, client.ObjectKeyFromObject(obj), current))
				current.Labels[clusterv1.ClusterNameLabel] = otherClusterName
				s.NoError(cl.Update(ctx, current))
				return cl.Update(ctx, obj, opts...)
			},
		}).Build()

	_, adopted, err := NewHelper(cl, s.cluster).AdoptIPAddressClaim(s.ctx, machine, def)
	s.True(apierrors.IsConflict(err), "expected resource-version conflict, got %v", err)
	s.False(adopted)
	actual := &ipamv1.IPAddressClaim{}
	s.NoError(cl.Get(s.ctx, client.ObjectKeyFromObject(claim), actual))
	s.Empty(actual.OwnerReferences)
	s.Equal(otherClusterName, actual.Labels[clusterv1.ClusterNameLabel])
	s.Equal(claim.Spec, actual.Spec)
	s.Equal(claim.Status, actual.Status)
}

func (s *IPAMTestSuite) Test_RecoveryRequiresOwningCluster() {
	for i, missingClaim := range []bool{false, true} {
		s.Run(fmt.Sprintf("missingClaim=%t", missingClaim), func() {
			machine := s.testMachine()
			def := s.testIPClaimDef(infrav1.DefaultNetworkDevice, fmt.Sprint(i), "test-cluster-v4-icip")
			claim := s.testIPAddressClaim(machine, def, "")
			claim.OwnerReferences = nil
			if !missingClaim {
				s.NoError(s.cl.Create(s.ctx, claim))
			}
			s.NoError(s.cl.Create(s.ctx, s.testIPAddress(claim.Namespace, claim.Name, def.PoolRef.Name)))
			cluster := s.cluster.DeepCopy()
			cluster.OwnerReferences = nil
			result, adopted, err := NewHelper(s.cl, cluster).AdoptIPAddressClaim(s.ctx, machine, def)
			s.EqualError(err, "ProxmoxCluster with OwnerReference but Cluster does not exist")
			s.False(adopted)
			s.Nil(result.Address)
		})
	}
}

func (s *IPAMTestSuite) Test_CreateIPAddressClaimPropagatesCreateFailure() {
	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))
	machine := s.testMachine()
	def := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	pool, err := s.helper.GetIPPool(s.ctx, def.PoolRef)
	s.NoError(err)
	createErr := apierrors.NewServiceUnavailable("claim creation unavailable")
	cl := fake.NewClientBuilder().WithScheme(s.cl.Scheme()).WithObjects(s.capiCluster, pool).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
				return createErr
			},
		}).Build()
	created, err := NewHelper(cl, s.cluster).CreateIPAddressClaimIfMissing(s.ctx, machine, def)
	s.ErrorIs(err, createErr)
	s.False(created)
	claims := &ipamv1.IPAddressClaimList{}
	s.NoError(cl.List(s.ctx, claims))
	s.Empty(claims.Items)
}

func (s *IPAMTestSuite) Test_CreateIPAddressClaimRejectsInvalidPrerequisites() {
	s.NoError(s.helper.CreateOrUpdateInClusterIPPool(s.ctx))
	tests := []struct {
		name    string
		mutate  func(*infrav1.ProxmoxCluster, *IPClaimDef)
		message string
	}{
		{
			name: "missing pool",
			mutate: func(_ *infrav1.ProxmoxCluster, def *IPClaimDef) {
				def.PoolRef.Name = otherPoolName
			},
			message: "unable to find InClusterIPPool " + otherPoolName,
		},
		{
			name: "missing cluster",
			mutate: func(cluster *infrav1.ProxmoxCluster, _ *IPClaimDef) {
				cluster.OwnerReferences[0].Name = otherClusterName
			},
			message: "not found",
		},
		{
			name: "no cluster owner",
			mutate: func(cluster *infrav1.ProxmoxCluster, _ *IPClaimDef) {
				cluster.OwnerReferences = nil
			},
			message: "ProxmoxCluster with OwnerReference but Cluster does not exist",
		},
		{
			name: "invalid offset",
			mutate: func(_ *infrav1.ProxmoxCluster, def *IPClaimDef) {
				def.Annotations[infrav1.ProxmoxPoolOffsetAnnotation] = "invalid"
			},
			message: "invalid",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			cluster := s.cluster.DeepCopy()
			def := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
			tt.mutate(cluster, &def)
			created, err := NewHelper(s.cl, cluster).CreateIPAddressClaimIfMissing(s.ctx, s.testMachine(), def)
			s.ErrorContains(err, tt.message)
			s.False(created)
			claims := &ipamv1.IPAddressClaimList{}
			s.NoError(s.cl.List(s.ctx, claims))
			s.Empty(claims.Items)
		})
	}
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimUnsafeOrphanIsConflict() {
	machine := s.testMachine()
	ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, "0", "test-cluster-v4-icip")
	orphanName := IPAddressFormat(machine.Name, infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix)
	orphan := s.testIPAddress(machine.Namespace, orphanName, "test-cluster-v4-icip")
	orphan.Spec.ClaimRef.Name = otherClaimName
	s.NoError(s.cl.Create(s.ctx, orphan))

	result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

	s.NoError(err)
	s.Equal(ClaimConflict, result.Status)
	s.Equal(ConflictAddressClaimRef, result.ConflictReason)
	s.Equal(orphanName, result.OrphanedAddress.Name)
}

func (s *IPAMTestSuite) Test_ResolveIPAddressClaimRejectsUnsafeOrphanProvenance() {
	tests := []struct {
		name   string
		reason IPAddressClaimConflictReason
		mutate func(*ipamv1.IPAddress)
	}{
		{
			name:   "pool reference",
			reason: ConflictAddressPoolRef,
			mutate: func(address *ipamv1.IPAddress) {
				address.Spec.PoolRef.Name = otherPoolName
			},
		},
		{
			name:   "cluster label",
			reason: ConflictAddressCluster,
			mutate: func(address *ipamv1.IPAddress) {
				address.Labels[clusterv1.ClusterNameLabel] = otherClusterName
			},
		},
		{
			name:   "controller owner",
			reason: ConflictAddressOwner,
			mutate: func(address *ipamv1.IPAddress) {
				address.OwnerReferences = []metav1.OwnerReference{{
					APIVersion: ipamv1.GroupVersion.String(),
					Kind:       "IPAddressClaim",
					Name:       "stale-claim",
					UID:        types.UID("stale-claim-uid"),
					Controller: new(true),
				}}
			},
		},
	}

	for i, tt := range tests {
		s.Run(tt.name, func() {
			machine := s.testMachine()
			offset := fmt.Sprint(i + 1)
			ipClaimDef := s.testIPClaimDef(infrav1.DefaultNetworkDevice, offset, "test-cluster-v4-icip")
			orphanName := IPAddressFormat(machine.Name, infrav1.DefaultNetworkDevice, i+1, infrav1.DefaultSuffix)
			orphan := s.testIPAddress(machine.Namespace, orphanName, "test-cluster-v4-icip")
			tt.mutate(orphan)
			s.NoError(s.cl.Create(s.ctx, orphan))

			result, err := s.helper.ResolveIPAddressClaim(s.ctx, machine, ipClaimDef)

			s.NoError(err)
			s.Equal(ClaimConflict, result.Status)
			s.Equal(tt.reason, result.ConflictReason)
			s.Equal(orphanName, result.OrphanedAddress.Name)
		})
	}
}

func (s *IPAMTestSuite) Test_GetIPAddressByPoolFiltersByPoolRefAndSorts() {
	poolRef := corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     GetInClusterIPPoolKind(),
	}

	matchingB := s.testIPAddress("test", "matching-b", poolRef.Name)
	matchingA := s.testIPAddress("test", "matching-a", poolRef.Name)
	wrongKind := s.testIPAddress("test", "wrong-kind", poolRef.Name)
	wrongKind.Spec.PoolRef.Kind = GetGlobalInClusterIPPoolKind()
	wrongGroup := s.testIPAddress("test", "wrong-group", poolRef.Name)
	wrongGroup.Spec.PoolRef.APIGroup = "other.ipam.example.com"
	otherPool := s.testIPAddress("test", otherPoolName, otherPoolName)
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

func (s *IPAMTestSuite) Test_HasDirectControllerOwnerReferenceRequiresExactMachineIdentity() {
	machine := s.testMachine()
	ownerRef := metav1.OwnerReference{
		APIVersion: infrav1.GroupVersion.String(),
		Kind:       infrav1.ProxmoxMachineKind,
		Name:       machine.Name,
		UID:        machine.UID,
		Controller: new(true),
	}

	s.True(hasDirectControllerOwnerReference([]metav1.OwnerReference{ownerRef}, machine))

	groupOnly := ownerRef
	groupOnly.APIVersion = infrav1.GroupVersion.Group
	s.True(hasDirectControllerOwnerReference([]metav1.OwnerReference{groupOnly}, machine))

	uidMismatch := ownerRef
	uidMismatch.UID = types.UID("other-uid")
	s.False(hasDirectControllerOwnerReference([]metav1.OwnerReference{uidMismatch}, machine))

	nameMismatch := ownerRef
	nameMismatch.Name = "other-machine"
	s.False(hasDirectControllerOwnerReference([]metav1.OwnerReference{nameMismatch}, machine))

	kindMismatch := ownerRef
	kindMismatch.Kind = "OtherMachine"
	s.False(hasDirectControllerOwnerReference([]metav1.OwnerReference{kindMismatch}, machine))

	apiGroupMismatch := ownerRef
	apiGroupMismatch.APIVersion = "other.infrastructure.cluster.x-k8s.io/v1alpha2"
	s.False(hasDirectControllerOwnerReference([]metav1.OwnerReference{apiGroupMismatch}, machine))

	nonController := ownerRef
	nonController.Controller = nil
	s.False(hasDirectControllerOwnerReference([]metav1.OwnerReference{nonController}, machine))
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
		Name:     otherPoolName,
		APIGroup: GetIPAMInClusterAPIGroup(),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesPoolRef(ip, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: new("other.ipam.example.com"),
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
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesClaimPoolRef(*claim, corev1.TypedLocalObjectReference{
		Name:     otherPoolName,
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesClaimPoolRef(*claim, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: new("other.ipam.example.com"),
		Kind:     GetInClusterIPPoolKind(),
	}))
	s.False(matchesClaimPoolRef(*claim, corev1.TypedLocalObjectReference{
		Name:     "test-cluster-v4-icip",
		APIGroup: new(ipamicv1.GroupVersion.String()),
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
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
	}
}

func (s *IPAMTestSuite) testIPClaimDef(device infrav1.NetName, offset, poolName string) IPClaimDef {
	return IPClaimDef{
		Device: device,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     poolName,
			APIGroup: new(ipamicv1.GroupVersion.String()),
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
			Name:      claimName,
			Namespace: machine.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
			Annotations: maps.Clone(ipClaimDef.Annotations),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       infrav1.ProxmoxMachineKind,
				Name:       machine.Name,
				UID:        machine.UID,
				Controller: new(true),
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

func (s *IPAMTestSuite) testIPAddress(namespace, name, poolName string, claimNames ...string) *ipamv1.IPAddress {
	claimName := name
	if len(claimNames) > 0 {
		claimName = claimNames[0]
	}
	return &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
		Spec: ipamv1.IPAddressSpec{
			ClaimRef: ipamv1.IPAddressClaimReference{Name: claimName},
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: ipamicv1.GroupVersion.Group,
				Kind:     GetInClusterIPPoolKind(),
				Name:     poolName,
			},
			Address: "192.0.2.1",
			Prefix:  new(int32(24)),
			Gateway: "192.0.2.254",
		},
	}
}

// Test_GetIPAddressByPool_APIGroupFilter verifies that GetIPAddressByPool accepts
// PoolRef.APIGroup in both "group" and "group/version" forms and rejects unrelated groups.
func (s *IPAMTestSuite) Test_GetIPAddressByPool_APIGroupFilter() {
	poolName := "filter-test-pool"

	// bare group — matches GetIPAMInClusterAPIVersion() directly after Cut.
	s.NoError(s.cl.Create(s.ctx, &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "addr-bare", Namespace: "test"},
		Spec: ipamv1.IPAddressSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: GetIPAMInClusterAPIVersion(),
				Kind:     GetInClusterIPPoolKind(),
				Name:     poolName,
			},
			ClaimRef: ipamv1.IPAddressClaimReference{Name: "claim-bare"},
			Address:  "192.0.2.1",
			Prefix:   new(int32(24)),
			Gateway:  "192.0.2.254",
		},
	}))

	// group/version — Cut extracts the group portion before the slash.
	s.NoError(s.cl.Create(s.ctx, &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "addr-groupversion", Namespace: "test"},
		Spec: ipamv1.IPAddressSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: *GetIPAMInClusterAPIGroup(),
				Kind:     GetInClusterIPPoolKind(),
				Name:     poolName,
			},
			ClaimRef: ipamv1.IPAddressClaimReference{Name: "claim-groupversion"},
			Address:  "192.0.2.2",
			Prefix:   new(int32(24)),
			Gateway:  "192.0.2.254",
		},
	}))

	// different group entirely — must be filtered out.
	s.NoError(s.cl.Create(s.ctx, &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "addr-other", Namespace: "test"},
		Spec: ipamv1.IPAddressSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: "other.example.io",
				Kind:     "OtherPool",
				Name:     poolName,
			},
			ClaimRef: ipamv1.IPAddressClaimReference{Name: "claim-other"},
			Address:  "192.0.2.3",
			Prefix:   new(int32(24)),
			Gateway:  "192.0.2.254",
		},
	}))

	result, err := s.helper.GetIPAddressByPool(s.ctx, corev1.TypedLocalObjectReference{
		APIGroup: GetIPAMInClusterAPIGroup(),
		Kind:     GetInClusterIPPoolKind(),
		Name:     poolName,
	})
	s.NoError(err)
	s.Len(result, 2)
	s.ElementsMatch([]string{result[0].Name, result[1].Name}, []string{"addr-bare", "addr-groupversion"})
}

// Test_GetInClusterPools_PoolRefKind verifies that the PoolRef.Kind returned by
// GetInClusterPools is populated from the constant rather than pool.TypeMeta.Kind,
// which is cleared by controller-runtime after every Get().
func (s *IPAMTestSuite) Test_GetInClusterPools_PoolRefKind() {
	pool := &ipamicv1.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zone-v4-pool",
			Namespace: "test",
		},
		Spec: ipamicv1.InClusterIPPoolSpec{
			Addresses: []string{"192.0.2.1-192.0.2.100"},
			Prefix:    24,
			Gateway:   "192.0.2.254",
		},
	}
	s.NoError(s.cl.Create(s.ctx, pool))

	defaultZone := "default"
	s.cluster.Status.InClusterZoneRef = []infrav1.InClusterZoneRef{
		{
			Zone:                 &defaultZone,
			InClusterIPPoolRefV4: &corev1.LocalObjectReference{Name: pool.Name},
		},
	}

	moxm := &infrav1.ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "test",
		},
		Spec: infrav1.ProxmoxMachineSpec{
			Network: &infrav1.NetworkSpec{},
		},
	}

	pools, err := s.helper.GetInClusterPools(s.ctx, moxm)
	s.NoError(err)
	s.NotNil(pools.IPv4)
	s.Equal(GetInClusterIPPoolKind(), pools.IPv4.PoolRef.Kind)
	s.Equal(pool.Name, pools.IPv4.PoolRef.Name)
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
			Prefix:  new(int32(24)),
			Gateway: "10.10.10.1",
		},
	}
}
