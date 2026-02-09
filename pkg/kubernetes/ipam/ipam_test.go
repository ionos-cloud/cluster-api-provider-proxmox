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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	. "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/consts"
)

type IPAMTestSuite struct {
	suite.Suite
	*require.Assertions
	ctx     context.Context
	cluster *infrav1.ProxmoxCluster
	cl      client.Client
	helper  *Helper
}

func TestIPAMTestSuite(t *testing.T) {
	suite.Run(t, new(IPAMTestSuite))
}

func (s *IPAMTestSuite) SetupTest() {
	s.cluster = getCluster()

	s.Assertions = require.New(s.T())
	scheme := scheme.Scheme

	s.NoError(clusterv1.AddToScheme(scheme))
	s.NoError(infrav1.AddToScheme(scheme))
	s.NoError(ipamicv1.AddToScheme(scheme))
	s.NoError(ipamv1.AddToScheme(scheme))

	fakeCl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(s.cluster).
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
		Device: ptr.To(infrav1.DefaultNetworkDevice),
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v4-icip",
			APIGroup: GetIpamInClusterAPIGroup(),
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
		Device: ptr.To(infrav1.DefaultNetworkDevice),
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-ippool-annotations",
			APIGroup: GetIpamInClusterAPIGroup(),
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
			ClaimRef: corev1.LocalObjectReference{
				Name: getCluster().GetName(),
			},
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(gvk.GroupVersion().String()),
				Kind:     gvk.Kind,
				Name:     "test-ippool-annotations",
			},
			Address: "10.10.11.11",
			Prefix:  24,
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

	device := ptr.To(infrav1.DefaultNetworkDevice)

	ipClaimDef := IPClaimDef{
		Device: device,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v4-icip",
			APIGroup: GetIpamInClusterAPIGroup(),
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

	additionalDevice := ptr.To("net1")

	ipClaimDef = IPClaimDef{
		Device: additionalDevice,
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v4-icip",
			APIGroup: GetIpamInClusterAPIGroup(),
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
		Device: ptr.To("net2"),
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-global-cluster-icip",
			APIGroup: GetIpamInClusterAPIGroup(),
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
			APIGroup: GetIpamInClusterAPIGroup(),
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
		Device: ptr.To(infrav1.DefaultNetworkDevice),
		PoolRef: corev1.TypedLocalObjectReference{
			Name:     "test-cluster-v4-icip",
			APIGroup: GetIpamInClusterAPIGroup(),
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

func getCluster() *infrav1.ProxmoxCluster {
	return &infrav1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PromoxCluster",
			APIVersion: infrav1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test",
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

func (s *IPAMTestSuite) dummyIPAddress(owner client.Object, poolName string) *ipamv1.IPAddress {
	gvk, err := apiutil.GVKForObject(new(ipamicv1.InClusterIPPool), s.cl.Scheme())
	if err != nil {
		panic(err)
	}
	return &ipamv1.IPAddress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IPAddress",
			APIVersion: "ipam.cluster.x-k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.GetName(),
			Namespace: owner.GetNamespace(),
		},
		Spec: ipamv1.IPAddressSpec{
			ClaimRef: corev1.LocalObjectReference{
				Name: owner.GetName(),
			},
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(gvk.GroupVersion().String()),
				Kind:     gvk.Kind,
				Name:     poolName,
			},
			Address: "10.10.10.11",
			Prefix:  24,
			Gateway: "10.10.10.1",
		},
	}
}
