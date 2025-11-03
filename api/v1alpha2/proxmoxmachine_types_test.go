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

package v1alpha2

import (
	"context"
	"strconv"

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
		Spec: ptr.To(ProxmoxMachineSpec{
			ProviderID:       ptr.To("proxmox://abcdef"),
			VirtualMachineID: ptr.To[int64](100),
			VirtualMachineCloneSpec: VirtualMachineCloneSpec{
				TemplateSource: TemplateSource{
					SourceNode: ptr.To("pve1"),
					TemplateID: ptr.To[int32](100),
				},
			},
			Disks: &Storage{
				BootVolume: &DiskSize{
					Disk:   "scsi0",
					SizeGB: 100,
				},
			},
		}),
	}
}

var _ = Describe("ProxmoxMachine Test", func() {
	AfterEach(func() {
		err := k8sClient.Delete(context.Background(), defaultMachine())
		Expect(client.IgnoreNotFound(err)).To(Succeed())
	})

	Context("VirtualMachineCloneSpec", func() {
		It("Should not allow specifying format if full clone is disabled", func() {
			dm := defaultMachine()
			dm.Spec.Format = ptr.To(TargetStorageFormatRaw)
			dm.Spec.Full = ptr.To(false)

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Must set full=true when specifying format")))
		})

		It("Should not allow specifying storage if full clone is disabled", func() {
			dm := defaultMachine()
			dm.Spec.Storage = ptr.To("local")
			dm.Spec.Full = ptr.To(false)

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Must set full=true when specifying storage")))
		})

		It("Should allow disabling full clone in absence of format and storage", func() {
			dm := defaultMachine()
			dm.Spec.Format = nil
			dm.Spec.Storage = nil
			dm.Spec.Full = ptr.To(false)

			Expect(k8sClient.Create(context.Background(), dm)).Should(Succeed())
		})

		It("Should disallow absence of SourceNode, TemplateID, and TemplateSelector", func() {
			dm := defaultMachine()
			dm.Spec.TemplateSource.SourceNode = nil
			dm.Spec.TemplateSource.TemplateID = nil
			dm.Spec.TemplateSelector = nil
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("must define either a SourceNode with a TemplateID or a TemplateSelector")))
		})

		It("Should not allow specifying TemplateSelector together with SourceNode and/or TemplateID", func() {
			dm := defaultMachine()
			dm.Spec.TemplateSelector = &TemplateSelector{MatchTags: []string{"test"}}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("must define either a SourceNode with a TemplateID or a TemplateSelector")))
		})

		It("Should not allow specifying TemplateSelector with empty MatchTags", func() {
			dm := defaultMachine()
			dm.Spec.TemplateSelector = &TemplateSelector{MatchTags: []string{}}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("spec.templateSelector.matchTags: Required value")))
		})

		It("Should only allow valid MatchTags", func() {
			testCases := []struct {
				tag          string
				expectErrror bool
				errorMessage string
			}{
				// Valid Tags
				{"valid_tag", false, ""},
				{"Valid-Tag", false, ""},
				{"valid.tag", false, ""},
				{"VALID+TAG", false, ""},
				{"123tag", false, ""},
				{"tag123", false, ""},
				{"tag_with-hyphen", false, ""},
				{"tag.with.plus+_and-hyphen", false, ""},
				{"_tag_with_underscore", false, ""},

				// Invalid Tags
				{"", true, "in body should match"},         // Empty string
				{"-invalid", true, "in body should match"}, // Starts with a hyphen
				{"+invalid", true, "in body should match"}, // Starts with a plus
				{".invalid", true, "in body should match"}, // Starts with a dot
				{" invalid", true, "in body should match"}, // Starts with a space
				{"invalid!", true, "in body should match"}, // Contains an exclamation mark
				{"invalid@", true, "in body should match"}, // Contains an at symbol
				{"invalid#", true, "in body should match"}, // Contains a hash symbol
				{"inval id", true, "in body should match"}, // Contains a whitespace
			}

			// Iterate through each test case
			for i, testCase := range testCases {
				// Create a new ProxmoxMachine object for each test case
				dm := defaultMachine()

				// Set the name of the machine to a unique value based on the test case index
				dm.ObjectMeta.Name = "test-machine-" + strconv.Itoa(i)

				// Set the template selector to match the tag from the test case
				dm.Spec.TemplateSource.SourceNode = nil
				dm.Spec.TemplateSource.TemplateID = nil
				dm.Spec.TemplateSelector = &TemplateSelector{MatchTags: []string{testCase.tag}}

				// Run test
				if !testCase.expectErrror {
					Expect(k8sClient.Create(context.Background(), dm)).To(Succeed())
				} else {
					Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring(testCase.errorMessage)))
				}
			}
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
				NetworkDevices: []NetworkDevice{{
					Bridge: ptr.To(""),
				}},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be at least 1 chars long")))
		})

		It("Should not allow net0 in additional network devices", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				NetworkDevices: []NetworkDevice{{
					Bridge: ptr.To("vmbr0"),
				}, {
					Name: ptr.To("net0"),
					InterfaceConfig: InterfaceConfig{
						IPPoolRef: []corev1.TypedLocalObjectReference{{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     "some-pool",
						}},
					},
				}},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("spec.network.networkDevices[1]: Duplicate value")))
		})

		It("Should only allow IPAM pool resources in IPPoolRef apiGroup", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				NetworkDevices: []NetworkDevice{{
					Bridge: ptr.To("vmbr0"),
					Name:   ptr.To("net1"),
					InterfaceConfig: InterfaceConfig{
						IPPoolRef: []corev1.TypedLocalObjectReference{{
							APIGroup: ptr.To("apps"),
							Name:     "some-app",
						}},
					},
				}},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipPoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io")))
		})

		It("Should only allow IPAM pool resources in IPPoolRef kind", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				NetworkDevices: []NetworkDevice{{
					Bridge: ptr.To("vmbr0"),
					Name:   ptr.To("net1"),
					InterfaceConfig: InterfaceConfig{
						IPPoolRef: []corev1.TypedLocalObjectReference{{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "ConfigMap",
							Name:     "some-app",
						}},
					},
				}},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipPoolRef allows either InClusterIPPool or GlobalInClusterIPPool")))
		})

		It("Should not allow machine with network device mtu less than 1", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				NetworkDevices: []NetworkDevice{{
					Bridge: ptr.To("vmbr0"),
					MTU:    ptr.To(int32(0)),
				}},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("invalid MTU value")))
		})

		It("Should not allow machine with network device mtu greater than 65520", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				NetworkDevices: []NetworkDevice{{
					Bridge: ptr.To("vmbr0"),
					MTU:    ptr.To(int32(65521)),
				}},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("invalid MTU value")))
		})

		/*It("Should only allow VRFS with a non kernel routing table ", func() {
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
		})*/

		/*It("Should only allow non kernel FIB rule priority", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				VirtualNetworkDevices: VirtualNetworkDevices{
					VRFs: []VRFDevice{{
						Name:  "vrf-blue",
						Table: 100,
						Routing: Routing{
							RoutingPolicy: []RoutingPolicySpec{{
								Priority: ptr.To(int64(32766)),
							}},
						},
					}},
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Cowardly refusing to insert FIB rule matching kernel rules")))
		})*/

		It("Should not allow machine with network device vlan equal to 0", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				NetworkDevices: []NetworkDevice{{
					Bridge: ptr.To("vmbr0"),
					VLAN:   ptr.To(int32(0)),
				}},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be greater than or equal to 1")))
		})

		It("Should not allow machine with network device vlan greater than 4094", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				NetworkDevices: []NetworkDevice{{
					Bridge: ptr.To("vmbr0"),
					VLAN:   ptr.To(int32(4095)),
				}},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be less than or equal to 4094")))
		})
	})

	Context("VMIDRange", func() {
		It("Should only allow spec.vmIDRange.start >= 100", func() {
			dm := defaultMachine()
			dm.Spec.VMIDRange = &VMIDRange{
				Start: 1,
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be greater than or equal to 100")))
		})
		It("Should only allow spec.vmIDRange.end >= 100", func() {
			dm := defaultMachine()
			dm.Spec.VMIDRange = &VMIDRange{
				End: 1,
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be greater than or equal to 100")))
		})
		It("Should only allow spec.vmIDRange.end >= spec.vmIDRange.start", func() {
			dm := defaultMachine()
			dm.Spec.VMIDRange = &VMIDRange{
				Start: 101,
				End:   100,
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be greater than or equal to start")))
		})
		It("Should only allow spec.vmIDRange.start if spec.vmIDRange.end is set", func() {
			dm := defaultMachine()
			dm.Spec.VMIDRange = &VMIDRange{
				Start: 100,
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("spec.vmIDRange.end: Required value")))
		})
		It("Should only allow spec.vmIDRange.end if spec.vmIDRange.start is set", func() {
			dm := defaultMachine()
			dm.Spec.VMIDRange = &VMIDRange{
				End: 100,
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("spec.vmIDRange.start: Required value")))
		})
	})

	Context("Tags", func() {
		It("should disallow invalid tags", func() {
			dm := defaultMachine()
			dm.Spec.Tags = []string{"foo=bar"}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Invalid value")))

			dm.Spec.Tags = []string{"foo$bar"}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Invalid value")))

			dm.Spec.Tags = []string{"foo^bar"}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Invalid value")))

			dm.Spec.Tags = []string{"foo bar"}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Invalid value")))

			dm.Spec.Tags = []string{"foo "}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Invalid value")))
		})

		It("Should not allow duplicated tags", func() {
			dm := defaultMachine()
			dm.Spec.Tags = []string{"foo", "bar", "foo"}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Duplicate value")))
			dm.Spec.Tags = []string{"foo", "bar"}
			Expect(k8sClient.Create(context.Background(), dm)).To(Succeed())
		})
	})
})
