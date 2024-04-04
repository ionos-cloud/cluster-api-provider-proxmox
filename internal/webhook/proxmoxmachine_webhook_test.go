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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
)

var _ = Describe("Controller Test", func() {
	g := NewWithT(GinkgoT())

	Context("create proxmox machine", func() {
		It("should disallow invalid network mtu", func() {
			machine := invalidMTUProxmoxMachine("test-machine")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("spec.network.default.mtu: Invalid value")))
		})

		It("should disallow invalid network vlan", func() {
			machine := invalidVLANProxmoxMachine("test-machine")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("spec.network.default.vlan: Invalid value")))
		})

		It("should disallow invalid network mtu for additional device", func() {
			machine := validProxmoxMachine("test-machine")
			machine.Spec.Network.AdditionalDevices[0].MTU = ptr.To(uint16(1000))
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("mtu must be at least 1280 or 1, but was 1000")))
		})

		It("should create a valid proxmox machine", func() {
			machine := validProxmoxMachine("test-machine")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(Succeed())
		})

		It("should disallow invalid network vlan for additional device", func() {
			machine := validProxmoxMachine("test-machine")
			machine.Spec.Network.AdditionalDevices[0].VLAN = ptr.To(uint16(0))
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("greater than or equal to 1")))
		})
	})

	Context("update proxmox cluster", func() {
		It("should disallow invalid network mtu", func() {
			clusterName := "test-cluster"
			machine := validProxmoxMachine(clusterName)
			g.Expect(k8sClient.Create(testEnv.GetContext(), &machine)).To(Succeed())

			g.Expect(k8sClient.Get(testEnv.GetContext(), client.ObjectKeyFromObject(&machine), &machine)).To(Succeed())
			machine.Spec.Network.Default.MTU = ptr.To(uint16(50))

			g.Expect(k8sClient.Update(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("spec.network.default.mtu: Invalid value")))
			machine.Spec.Network.Default.VLAN = ptr.To(uint16(0))

			g.Expect(k8sClient.Update(testEnv.GetContext(), &machine)).To(MatchError(ContainSubstring("spec.network.default.vlan: Invalid value")))

			g.Eventually(func(g Gomega) {
				g.Expect(client.IgnoreNotFound(k8sClient.Delete(testEnv.GetContext(), &machine))).To(Succeed())
			}).WithTimeout(time.Second * 10).
				WithPolling(time.Second).
				Should(Succeed())
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
				SourceNode: "pve",
			},
			NumSockets: 1,
			NumCores:   1,
			MemoryMiB:  1024,
			Disks: &infrav1.Storage{
				BootVolume: &infrav1.DiskSize{
					Disk:   "scsi[0]",
					SizeGB: 10,
				},
			},
			Network: &infrav1.NetworkSpec{
				Default: &infrav1.NetworkDevice{
					Bridge: "vmbr1",
					Model:  ptr.To("virtio"),
					MTU:    ptr.To(uint16(1500)),
					VLAN:   ptr.To(uint16(100)),
				},
				AdditionalDevices: []infrav1.AdditionalNetworkDevice{
					{
						Name: "net1",
						NetworkDevice: infrav1.NetworkDevice{
							Bridge: "vmbr2",
							Model:  ptr.To("virtio"),
							MTU:    ptr.To(uint16(1500)),
							VLAN:   ptr.To(uint16(100)),
						},
						InterfaceConfig: infrav1.InterfaceConfig{
							IPv4PoolRef: &corev1.TypedLocalObjectReference{
								Name:     "simple-pool",
								Kind:     "InClusterIPPool",
								APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							},
						},
					},
				},
			},
		},
	}
}

func invalidMTUProxmoxMachine(name string) infrav1.ProxmoxMachine {
	machine := validProxmoxMachine(name)
	machine.Spec.Network.Default = &infrav1.NetworkDevice{
		Bridge: "vmbr1",
		Model:  ptr.To("virtio"),
		MTU:    ptr.To(uint16(50)),
	}
	return machine
}

func invalidVLANProxmoxMachine(name string) infrav1.ProxmoxMachine {
	machine := validProxmoxMachine(name)
	machine.Spec.Network.Default = &infrav1.NetworkDevice{
		Bridge: "vmbr1",
		Model:  ptr.To("virtio"),
		VLAN:   ptr.To(uint16(0)),
	}
	return machine
}
