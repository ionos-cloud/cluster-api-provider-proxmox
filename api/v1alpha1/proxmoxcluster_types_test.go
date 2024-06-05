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

package v1alpha1

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestUpdateNodeLocation(t *testing.T) {
	cl := ProxmoxCluster{
		Status: ProxmoxClusterStatus{},
	}

	res := cl.UpdateNodeLocation("new", "n1", false)
	require.NotNil(t, cl.Status.NodeLocations)
	require.Len(t, cl.Status.NodeLocations.Workers, 1)
	require.True(t, res)

	locs := &NodeLocations{
		Workers: []NodeLocation{
			{
				Machine: corev1.LocalObjectReference{Name: "m1"},
				Node:    "n1",
			},
			{
				Machine: corev1.LocalObjectReference{Name: "m2"},
				Node:    "n2",
			},
			{
				Machine: corev1.LocalObjectReference{Name: "m3"},
				Node:    "n3",
			},
		},
	}

	cl.Status.NodeLocations = locs

	res = cl.UpdateNodeLocation("m1", "n2", false)
	require.True(t, res)
	require.Len(t, cl.Status.NodeLocations.Workers, 3)
	require.Equal(t, cl.Status.NodeLocations.Workers[0].Node, "n2")

	res = cl.UpdateNodeLocation("m4", "n4", false)
	require.True(t, res)
	require.Len(t, cl.Status.NodeLocations.Workers, 4)
	require.Equal(t, cl.Status.NodeLocations.Workers[3].Node, "n4")

	res = cl.UpdateNodeLocation("m2", "n2", false)
	require.False(t, res)
	require.Len(t, cl.Status.NodeLocations.Workers, 4)
}

func defaultCluster() *ProxmoxCluster {
	return &ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: ProxmoxClusterSpec{
			IPv4Config: &IPConfigSpec{
				Addresses: []string{"10.0.0.0/24"},
				Prefix:    24,
				Gateway:   "10.0.0.254",
				Metric:    func() *uint32 { var a uint32 = 123; return &a }(),
			},
			DNSServers: []string{"1.2.3.4"},
			CloneSpec: &ProxmoxClusterCloneSpec{
				ProxmoxMachineSpec: map[string]ProxmoxMachineSpec{
					"controlPlane": {
						VirtualMachineCloneSpec: VirtualMachineCloneSpec{
							SourceNode: "pve1",
						},
					},
				},
			},
		},
	}
}

var _ = Describe("ProxmoxCluster Test", func() {
	AfterEach(func() {
		err := k8sClient.Delete(context.Background(), defaultCluster())
		Expect(client.IgnoreNotFound(err)).To(Succeed())
	})

	Context("IPv4Config", func() {
		It("Should not allow empty addresses", func() {
			dc := defaultCluster()
			dc.Spec.IPv4Config.Addresses = []string{}

			Expect(k8sClient.Create(context.Background(), dc)).Should(MatchError(ContainSubstring("IPv4Config addresses must be provided")))
		})

		It("Should not allow prefix higher than 128", func() {
			dc := defaultCluster()
			dc.Spec.IPv4Config.Prefix = 129

			Expect(k8sClient.Create(context.Background(), dc)).Should(MatchError(ContainSubstring("should be less than or equal to 128")))
		})

		It("Should not allow empty ip config", func() {
			dc := defaultCluster()
			dc.Spec.IPv6Config = nil
			dc.Spec.IPv4Config = nil
			Expect(k8sClient.Create(context.Background(), dc)).Should(MatchError(ContainSubstring("at least one ip config must be set")))
		})
	})

	It("Should not allow empty DNS servers", func() {
		dc := defaultCluster()
		dc.Spec.DNSServers = []string{}

		Expect(k8sClient.Create(context.Background(), dc)).Should(MatchError(ContainSubstring("should have at least 1 items")))
	})

	It("Should allow creating valid clusters", func() {
		Expect(k8sClient.Create(context.Background(), defaultCluster())).To(Succeed())
	})

	Context("CloneSpecs", func() {
		It("Should not allow Cluster without ControlPlane nodes", func() {
			dc := defaultCluster()
			dc.Spec.CloneSpec.ProxmoxMachineSpec = map[string]ProxmoxMachineSpec{}

			Expect(k8sClient.Create(context.Background(), dc)).Should(MatchError(ContainSubstring("control plane")))
		})
	})

	Context("IPV6Config", func() {
		It("Should not allow empty addresses", func() {
			dc := defaultCluster()
			dc.Spec.IPv6Config = &IPConfigSpec{
				Addresses: []string{},
				Prefix:    0,
				Gateway:   "",
			}
			Expect(k8sClient.Create(context.Background(), dc)).Should(MatchError(ContainSubstring("IPv6Config addresses must be provided")))
		})

		It("Should not allow prefix higher than 128", func() {
			dc := defaultCluster()
			dc.Spec.IPv6Config = &IPConfigSpec{
				Addresses: []string{},
				Prefix:    129,
				Gateway:   "",
			}

			Expect(k8sClient.Create(context.Background(), dc)).Should(MatchError(ContainSubstring("should be less than or equal to 128")))
		})
	})
})

func TestRemoveNodeLocation(t *testing.T) {
	cl := ProxmoxCluster{
		Status: ProxmoxClusterStatus{NodeLocations: &NodeLocations{
			Workers: []NodeLocation{
				{
					Machine: corev1.LocalObjectReference{Name: "m1"},
					Node:    "n1",
				},
				{
					Machine: corev1.LocalObjectReference{Name: "m2"},
					Node:    "n2",
				},
				{
					Machine: corev1.LocalObjectReference{Name: "m3"},
					Node:    "n3",
				},
			},
		}},
	}

	cl.RemoveNodeLocation("m1", false)
	require.NotNil(t, cl.Status.NodeLocations)
	require.Len(t, cl.Status.NodeLocations.Workers, 2)

	cl.RemoveNodeLocation("m1", false)
	require.Len(t, cl.Status.NodeLocations.Workers, 2)
	require.Equal(t, cl.Status.NodeLocations.Workers[0].Node, "n2")

	cl.UpdateNodeLocation("m4", "n4", true)
	require.Len(t, cl.Status.NodeLocations.ControlPlane, 1)

	cl.RemoveNodeLocation("m4", true)
	require.Len(t, cl.Status.NodeLocations.ControlPlane, 0)
}

func TestSetInClusterIPPoolRef(t *testing.T) {
	cl := defaultCluster()

	cl.SetInClusterIPPoolRef(nil)
	require.Nil(t, cl.Status.InClusterIPPoolRef)

	pool := &ipamicv1.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: ipamicv1.InClusterIPPoolSpec{
			Addresses: []string{"10.10.10.2/24"},
			Prefix:    24,
			Gateway:   "10.10.10.1",
		},
	}

	cl.SetInClusterIPPoolRef(pool)
	require.Equal(t, cl.Status.InClusterIPPoolRef[0].Name, pool.GetName())

	cl.SetInClusterIPPoolRef(pool)
	require.Equal(t, cl.Status.InClusterIPPoolRef[0].Name, pool.GetName())
}
