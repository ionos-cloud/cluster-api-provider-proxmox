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

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func defaultMachine() *ProxmoxMachine {
	return &ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: ProxmoxMachineSpec{
			VirtualMachineCloneSpec: VirtualMachineCloneSpec{
				SourceNode: "pve1",
			},
			ProviderID:       ptr.To("proxmox://abcdef"),
			VirtualMachineID: ptr.To[int64](100),
			Disks: &Storage{
				BootVolume: &DiskSize{
					Disk:   "scsi0",
					SizeGB: 100,
				},
			},
		},
	}
}

var _ = Describe("ProxmoxMachine Test", func() {
	AfterEach(func() {
		err := k8sClient.Delete(context.Background(), defaultMachine())
		Expect(client.IgnoreNotFound(err)).To(Succeed())
	})

	Context("VirtualMachineCloneSpec", func() {
		It("Should not allow empty source node", func() {
			dm := defaultMachine()
			dm.Spec.SourceNode = ""

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be at least 1 chars long")))
		})

		It("Should not allow specifying format if full clone is disabled", func() {
			dm := defaultMachine()
			dm.Spec.Full = ptr.To(false)

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Must set full=true when specifying format")))
		})
	})

	Context("Disks", func() {
		It("Should not allow updates to disks", func() {
			dm := defaultMachine()
			Expect(k8sClient.Create(context.Background(), dm)).To(Succeed())
			dm.Spec.Disks.BootVolume.SizeGB = 50
			Expect(k8sClient.Update(context.Background(), dm)).Should(MatchError(ContainSubstring("is immutable")))
		})

		It("Should not allow negative or less than minimum values", func() {
			dm := defaultMachine()

			dm.Spec.Disks.BootVolume.SizeGB = -10
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("greater than or equal to 5")))

			dm.Spec.Disks.BootVolume.SizeGB = 4
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("greater than or equal to 5")))
		})
	})

	Context("Network", func() {
		It("Should set default bridge", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				Default: &NetworkDevice{
					Bridge: "",
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be at least 1 chars long")))
		})

		It("Should not allow net0 in additional network devices", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				Default: &NetworkDevice{
					Bridge: "vmbr0",
				},
				AdditionalDevices: []AdditionalNetworkDevice{{
					NetworkDevice: NetworkDevice{},
					Name:          "net0",
					IPv4PoolRef: &corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "InClusterIPPool",
						Name:     "some-pool",
					},
				},
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be at least 1 chars long")))
		})

		It("Should only allow IPAM pool resources in IPv4PoolRef apiGroup", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{{
					NetworkDevice: NetworkDevice{},
					Name:          "net1",
					IPv4PoolRef: &corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("apps"),
						Name:     "some-app",
					},
				},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipv4PoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io")))
		})

		It("Should only allow IPAM pool resources in IPv4PoolRef kind", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{{
					NetworkDevice: NetworkDevice{},
					Name:          "net1",
					IPv4PoolRef: &corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "ConfigMap",
						Name:     "some-app",
					},
				},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipv4PoolRef allows either InClusterIPPool or GlobalInClusterIPPool")))
		})

		It("Should only allow IPAM pool resources in IPv6PoolRef apiGroup", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{{
					NetworkDevice: NetworkDevice{},
					Name:          "net1",
					IPv6PoolRef: &corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("apps"),
						Name:     "some-app",
					},
				},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipv6PoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io")))
		})

		It("Should only allow IPAM pool resources in IPv6PoolRef kind", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{{
					NetworkDevice: NetworkDevice{},
					Name:          "net1",
					IPv6PoolRef: &corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "ConfigMap",
						Name:     "some-app",
					},
				},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipv6PoolRef allows either InClusterIPPool or GlobalInClusterIPPool")))
		})

		It("Should only allow Machine with additional devices with at least a pool ref", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{{
					NetworkDevice: NetworkDevice{},
					Name:          "net1",
				},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("at least one pool reference must be set, either ipv4PoolRef or ipv6PoolRef")))
		})

		It("Should not allow machine with network device mtu less than 1", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				Default: &NetworkDevice{
					Bridge: "vmbr0",
					MTU:    ptr.To(uint16(0)),
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be greater than or equal to 1")))
		})

		It("Should not allow machine with network device mtu greater than 65520", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				Default: &NetworkDevice{
					Bridge: "vmbr0",
					MTU:    ptr.To(uint16(65521)),
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be less than or equal to 65520")))
		})

		It("Should only allow VRFS with a non kernel routing table ", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				VirtualNetworkDevices: VirtualNetworkDevices{
					VRFs: []VRFDevice{{
						Name:  "vrf-blue",
						Table: 254,
					}},
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Cowardly refusing to insert l3mdev rules into kernel tables")))
		})

		It("Should only allow non kernel FIB rule priority", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				VirtualNetworkDevices: VirtualNetworkDevices{
					VRFs: []VRFDevice{{
						Name:  "vrf-blue",
						Table: 100,
						RoutingPolicy: []RoutingPolicySpec{{
							Priority: 32766,
						}},
					}},
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Cowardly refusing to insert fib rule matching kernel rules")))
		})
	})
})
