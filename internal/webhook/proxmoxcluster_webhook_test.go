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

package webhook

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

var _ = Describe("Controller Test", func() {
	g := NewWithT(GinkgoT())

	Context("create proxmox cluster", func() {
		It("should disallow endpoint IP to intersect with node IPs", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("addresses may not contain the endpoint IP")))
		})

		It("should disallow cluster without any IP pool config", func() {
			cluster := validProxmoxCluster("test-cluster")
			cluster.Spec.IPv4Config = nil
			cluster.SetName("test-invalid-cluster")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("at least one ip config must be set")))
		})

		It("should disallow invalid endpoint FQDN", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			cluster.Spec.ControlPlaneEndpoint.Host = "_this.is.a.txt.record"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("provided endpoint address is not a valid IP or FQDN")))
		})

		It("should disallow invalid endpoint short hostname", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			cluster.Spec.ControlPlaneEndpoint.Host = "invalid-"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("provided endpoint address is not a valid IP or FQDN")))
		})

		It("should allow valid endpoint FQDN", func() {
			cluster := validProxmoxCluster("succeed-test-cluster-with-fqdn")
			cluster.Spec.ControlPlaneEndpoint.Host = "host.example.com"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(Succeed())
		})

		It("should allow valid upper case endpoint FQDN", func() {
			cluster := validProxmoxCluster("succeed-test-cluster-with-uppercase-fqdn")
			cluster.Spec.ControlPlaneEndpoint.Host = "HOST.EXAMPLE.COM"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(Succeed())
		})

		It("should allow valid IPv4 endpoint", func() {
			cluster := validProxmoxCluster("succeed-test-cluster-with-ipv4")
			cluster.Spec.ControlPlaneEndpoint.Host = "127.0.0.1"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(Succeed())
		})

		It("should allow valid IPv6 endpoint", func() {
			cluster := validProxmoxCluster("succeed-test-cluster-with-ipv6")
			cluster.Spec.ControlPlaneEndpoint.Host = "::1"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(Succeed())
		})

		It("should disallow invalid IPv4 addresses", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			cluster.Spec.IPv4Config.Addresses = []string{"invalid"}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("provided addresses are not valid IP addresses, ranges or CIDRs")))
		})

		It("should disallow invalid IPv6 addresses", func() {
			cluster := validProxmoxCluster("test-cluster")
			cluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
				Addresses: []string{"invalid"},
				Prefix:    64,
				Gateway:   "2001:db8::1",
			}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("provided addresses are not valid IP addresses, ranges or CIDRs")))
		})

		It("should disallow endpoint IP to intersect with node IPs", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			cluster.Spec.ControlPlaneEndpoint.Host = "2001:db8::1"
			cluster.Spec.IPv6Config = &infrav1.IPConfigSpec{
				Addresses: []string{"2001:db8::/64"},
				Prefix:    64,
				Gateway:   "2001:db8::1",
			}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("addresses may not contain the endpoint IP")))
		})

		It("should disallow clusterClassSpec without controlPlane", func() {
			cluster := validProxmoxCluster("test-cluster")
			cluster.Spec.CloneSpec.ProxmoxClusterClassSpec[0].MachineType = "coward"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("machineSpec must contain an entry with machineType 'controlPlane'")))
		})
	})

	Context("update proxmox cluster", func() {
		It("should disallow new endpoint IP to intersect with node IPs", func() {
			clusterName := "test-cluster"
			cluster := validProxmoxCluster(clusterName)
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(Succeed())

			g.Expect(k8sClient.Get(testEnv.GetContext(), client.ObjectKeyFromObject(&cluster), &cluster)).To(Succeed())
			cluster.Spec.ControlPlaneEndpoint.Host = "10.10.10.2"

			g.Expect(k8sClient.Update(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("addresses may not contain the endpoint IP")))

			g.Eventually(func(g Gomega) {
				g.Expect(client.IgnoreNotFound(k8sClient.Delete(testEnv.GetContext(), &cluster))).To(Succeed())
			}).WithTimeout(time.Second * 10).
				WithPolling(time.Second).
				Should(Succeed())
		})
	})
})

func validProxmoxCluster(name string) infrav1.ProxmoxCluster {
	return infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: infrav1.ProxmoxClusterSpec{
			ControlPlaneEndpoint: &clusterv1.APIEndpoint{
				Host: "10.10.10.1",
				Port: 6443,
			},
			IPv4Config: &infrav1.IPConfigSpec{
				Addresses: []string{
					"10.10.10.2-10.10.10.10",
				},
				Gateway: "10.10.10.1",
				Prefix:  24,
			},
			DNSServers: []string{"8.8.8.8", "8.8.4.4"},
			CloneSpec: &infrav1.ProxmoxClusterCloneSpec{
				ProxmoxClusterClassSpec: []infrav1.ProxmoxClusterClassSpec{{
					MachineType: "controlPlane",
					ProxmoxMachineSpec: infrav1.ProxmoxMachineSpec{
						Network: &infrav1.NetworkSpec{
							NetworkDevices: []infrav1.NetworkDevice{{
								Name:   ptr.To("net0"),
								Bridge: ptr.To("vmbr0"),
							}},
						},
					},
				}},
			},
		},
	}
}

func invalidProxmoxCluster(name string) infrav1.ProxmoxCluster {
	cl := validProxmoxCluster(name)
	cl.Spec.ControlPlaneEndpoint = &clusterv1.APIEndpoint{
		Host: "10.10.10.2",
		Port: 6443,
	}

	return cl
}
