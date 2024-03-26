/*
Copyright 2023 IONOS Cloud.

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

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

		It("should disallow invalid/non-existing endpoint FQDN", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			cluster.Spec.ControlPlaneEndpoint.Host = "this.does.not.exist.ionos.com"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("provided endpoint address is not a valid IP or FQDN")))
		})

		It("should disallow invalid endpoint IP", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			cluster.Spec.ControlPlaneEndpoint.Host = "invalid"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("provided endpoint address is not a valid IP")))
		})

		It("should allow valid endpoint from FQDN", func() {
			cluster := validProxmoxCluster("succeed-test-cluster-with-fqdn")
			cluster.Spec.ControlPlaneEndpoint.Host = "ionos.com"
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(Succeed())
		})

		It("should disallow invalid endpoint IP + port combination", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			cluster.Spec.ControlPlaneEndpoint.Host = "127.0.0.1"
			cluster.Spec.ControlPlaneEndpoint.Port = 69000
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("provided endpoint is not in a valid IP and port format")))
		})

		It("should disallow invalid IPV4 IPs", func() {
			cluster := invalidProxmoxCluster("test-cluster")
			cluster.Spec.IPv4Config.Addresses = []string{"invalid"}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cluster)).To(MatchError(ContainSubstring("provided addresses are not valid IP addresses, ranges or CIDRs")))
		})

		It("should disallow invalid IPV6 IPs", func() {
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
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
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
		},
	}
}

func invalidProxmoxCluster(name string) infrav1.ProxmoxCluster {
	cl := validProxmoxCluster(name)
	cl.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
		Host: "10.10.10.2",
		Port: 6443,
	}

	return cl
}
