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

package webhook

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	. "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/consts"
)

var _ = Describe("Controller Test", func() {
	g := NewWithT(GinkgoT())

	Context("create proxmox machine", func() {
		It("should disallow invalid network mtu", func() {
			machine := invalidMTUProxmoxMachine("test-machine")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("invalid MTU value")))
		})

		It("should disallow invalid network vlan", func() {
			machine := invalidVLANProxmoxMachine("test-machine")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("spec.network.networkDevices[0].vlan: Invalid value")))
		})

		It("should disallow invalid network mtu for additional device", func() {
			machine := validProxmoxMachine("test-machine")
			machine.Spec.Network.NetworkDevices[0].MTU = ptr.To(int32(1000))
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("mtu must be at least 1280 or 1, but was 1000")))
		})

		It("should create a valid proxmox machine", func() {
			machine := validProxmoxMachine("test-machine")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(Succeed())
		})

		It("should disallow invalid network vlan", func() {
			machine := validProxmoxMachine("test-machine")
			machine.Spec.Network.NetworkDevices[0].VLAN = ptr.To(int32(0))
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("greater than or equal to 1")))
		})

		It("should disallow invalid link mtu", func() {
			machine := validProxmoxMachine("test-machine")
			machine.Spec.Network.NetworkDevices[0].LinkMTU = ptr.To(int32(1000))
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("mtu must be at least 1280, but was 1000")))
		})

		It("should disallow conflicting l3mdev/routing policy table", func() {
			machine := validProxmoxMachine("test-machine")
			*machine.Spec.Network.VirtualNetworkDevices.VRFs[0].Routing.RoutingPolicy[0].Table = 667
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("VRF vrf-green: device/rule routing table mismatch 665 != 667")))
		})

		It("should disallow routing policy without table", func() {
			machine := validProxmoxMachine("test-machine")
			machine.Spec.Network.NetworkDevices[0].InterfaceConfig.Routing.RoutingPolicy = []infrav1.RoutingPolicySpec{{}}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("routing policy [0] requires a table")))
		})

		It("should accept MTU=1 (inherit bridge MTU) on default device", func() {
			machine := validProxmoxMachine("net-inherit-mtu-default")
			machine.Spec.Network.NetworkDevices[0].MTU = ptr.To(int32(1))
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(Succeed())
		})

		It("should disallow having two devices named net0", func() {
			machine := validProxmoxMachine("net-additional-name-net0")
			machine.Spec.Network.NetworkDevices[1].Name = ptr.To("net0")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("Duplicate value")))
		})

		It("should reject unknown network model values", func() {
			machine := validProxmoxMachine("net-unknown-model")
			machine.Spec.Network.NetworkDevices[0].Model = ptr.To("foo")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("Unsupported value")))
		})

		It("should reject too large MTU", func() {
			machine := validProxmoxMachine("net-mtu-too-large")
			machine.Spec.Network.NetworkDevices[0].MTU = ptr.To(int32(65521))
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("invalid MTU value")))
		})

		It("should reject FIB rule priorities that match kernel rules", func() {
			machine5 := validProxmoxMachine("net-routingpolicy-priority-32765")
			machine5.Spec.Network.NetworkDevices[0].RoutingPolicy = []infrav1.RoutingPolicySpec{{Priority: ptr.To(int64(32765))}}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine5)).To(MatchError(ContainSubstring("Cowardly refusing to insert FIB rule matching kernel rules")))

			machine6 := validProxmoxMachine("net-routingpolicy-priority-32766")
			machine6.Spec.Network.NetworkDevices[0].RoutingPolicy = []infrav1.RoutingPolicySpec{{Priority: ptr.To(int64(32766))}}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine6)).To(MatchError(ContainSubstring("Cowardly refusing to insert FIB rule matching kernel rules")))

		})

		It("should reject VRF device tables that target kernel tables", func() {
			machine4 := validProxmoxMachine("net-vrf-table-254")
			machine4.Spec.Network.VirtualNetworkDevices.VRFs = []infrav1.VRFDevice{{
				Name:  "vrf-blue",
				Table: 254,
			}}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine4)).To(MatchError(ContainSubstring("Cowardly refusing to insert l3mdev rules into kernel tables")))

			machine5 := validProxmoxMachine("net-vrf-table-255")
			machine5.Spec.Network.VirtualNetworkDevices.VRFs = []infrav1.VRFDevice{{
				Name:  "vrf-blue",
				Table: 255,
			}}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine5)).To(MatchError(ContainSubstring("Cowardly refusing to insert l3mdev rules into kernel tables")))
		})

		It("should error with multiple default ipv4 pool tags", func() {
			machine := validProxmoxMachine("multiple-default-v4-pools")
			machine.Spec.Network.NetworkDevices[0].DefaultIPv4 = ptr.To(true)
			machine.Spec.Network.NetworkDevices[1].DefaultIPv4 = ptr.To(true)
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("More than one default IPv4/IPv6 interface in NetworkDevices")))
		})

		It("should error with multiple default ipv6 pool tags", func() {
			machine := validProxmoxMachine("multiple-default-v6-pools")
			machine.Spec.Network.NetworkDevices[0].DefaultIPv6 = ptr.To(true)
			machine.Spec.Network.NetworkDevices[1].DefaultIPv6 = ptr.To(true)
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("More than one default IPv4/IPv6 interface in NetworkDevices")))
		})

		It("should not add default ipv4/ipv6 pool tags when defined", func() {
			machine := validProxmoxMachine("default-pools-exist")
			machine.Spec.Network.NetworkDevices[1].DefaultIPv4 = ptr.To(true)
			machine.Spec.Network.NetworkDevices[1].DefaultIPv6 = ptr.To(true)
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(Succeed())
			g.Expect(machine.Spec.Network.NetworkDevices[0].DefaultIPv4).To(BeNil())
			g.Expect(machine.Spec.Network.NetworkDevices[0].DefaultIPv6).To(BeNil())
		})

		It("should add default ipv4/ipv6 pool tags", func() {
			machine := validProxmoxMachine("no-default-pools")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(Succeed())
			g.Expect(*machine.Spec.Network.NetworkDevices[0].DefaultIPv4).To(Equal(true))
			g.Expect(*machine.Spec.Network.NetworkDevices[0].DefaultIPv6).To(Equal(true))
		})

		It("should not allow non consecutive network interface names ", func() {
			machine := validProxmoxMachine("non-consecutive-netname")
			machine.Spec.Network.NetworkDevices[1].Name = ptr.To("net2")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("consecutive")))
		})
	})

	Context("update proxmox cluster", func() {
		It("should disallow invalid network mtu", func() {
			clusterName := "test-cluster"
			machine := validProxmoxMachine(clusterName)
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(Succeed())

			g.Expect(k8sClient.Get(testEnv.GetContext(), client.ObjectKeyFromObject(&machine), &machine)).To(Succeed())
			machine.Spec.Network.NetworkDevices[0].MTU = ptr.To(int32(50))

			g.Expect(k8sClient.Update(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("invalid MTU value")))
			machine.Spec.Network.NetworkDevices[0].VLAN = ptr.To(int32(0))

			g.Expect(k8sClient.Update(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("invalid MTU value")))

			g.Eventually(func(g Gomega) {
				g.Expect(client.IgnoreNotFound(k8sClient.Delete(testEnv.GetContext(), &machine))).To(Succeed())
			}).WithTimeout(time.Second * 10).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("should not allow updates on tags", func() {
			machine := validProxmoxMachine("test-machine-tags")
			machine.Spec.Tags = []string{"foo_bar"}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(Succeed())

			machine.Spec.Tags = []string{"foobar", "barfoo"}
			g.Expect(k8sClient.Update(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("tags are immutable")))
		})
	})
})

func validProxmoxMachine(name string) infrav1.ProxmoxMachine {
	return infrav1.ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: infrav1.ProxmoxMachineSpec{
			VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
				TemplateSource: infrav1.TemplateSource{
					SourceNode: ptr.To("pve"),
					TemplateID: ptr.To(int32(100)),
				},
			},
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(1)),
			MemoryMiB:  ptr.To(int32(1024)),
			Disks: &infrav1.Storage{
				BootVolume: &infrav1.DiskSize{
					Disk:   "scsi[0]",
					SizeGB: 10,
				},
			},
			Network: &infrav1.NetworkSpec{
				NetworkDevices: []infrav1.NetworkDevice{{
					Name:   ptr.To("net0"),
					Bridge: ptr.To("vmbr1"),
					Model:  ptr.To("virtio"),
					MTU:    ptr.To(int32(1500)),
					VLAN:   ptr.To(int32(100)),
				}, {
					Name:   ptr.To("net1"),
					Bridge: ptr.To("vmbr2"),
					Model:  ptr.To("virtio"),
					MTU:    ptr.To(int32(1500)),
					VLAN:   ptr.To(int32(100)),
					InterfaceConfig: infrav1.InterfaceConfig{
						IPPoolRef: []corev1.TypedLocalObjectReference{{
							Name:     "simple-pool",
							Kind:     InClusterIPPool,
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						}},
						Routing: infrav1.Routing{
							RoutingPolicy: []infrav1.RoutingPolicySpec{{
								Table: ptr.To(int32(665)),
							}},
						},
					},
				}},
				VirtualNetworkDevices: infrav1.VirtualNetworkDevices{
					VRFs: []infrav1.VRFDevice{{
						Table: 665,
						Name:  "vrf-green",
						Routing: infrav1.Routing{
							RoutingPolicy: []infrav1.RoutingPolicySpec{{
								Table: ptr.To(int32(665)),
							}},
						}},
					},
				},
			},
		},
	}
}

func invalidMTUProxmoxMachine(name string) infrav1.ProxmoxMachine {
	machine := validProxmoxMachine(name)
	machine.Spec.Network.NetworkDevices = []infrav1.NetworkDevice{{
		Name:   ptr.To("net0"),
		Bridge: ptr.To("vmbr1"),
		Model:  ptr.To("virtio"),
		MTU:    ptr.To(int32(50)),
	}}
	return machine
}

func invalidVLANProxmoxMachine(name string) infrav1.ProxmoxMachine {
	machine := validProxmoxMachine(name)
	machine.Spec.Network.NetworkDevices = []infrav1.NetworkDevice{{
		Name:   ptr.To("net0"),
		Bridge: ptr.To("vmbr1"),
		Model:  ptr.To("virtio"),
		VLAN:   ptr.To(int32(0)),
	}}
	return machine
}
