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

package v1alpha1

import (
	"context"
	"encoding/json"
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
		Spec: ProxmoxMachineSpec{
			ProviderID:       ptr.To("proxmox://abcdef"),
			VirtualMachineID: ptr.To[int64](100),
			VirtualMachineCloneSpec: VirtualMachineCloneSpec{
				TemplateSource: TemplateSource{
					SourceNode: "pve1",
					TemplateID: ptr.To[int32](100),
				},
			},
			Disks: &Storage{
				BootVolume: &DiskSpec{
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
		It("Should not allow specifying format if full clone is disabled", func() {
			dm := defaultMachine()
			dm.Spec.Format = ptr.To(TargetFileStorageFormatRaw)
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

		It("Should disallow absence of SourceNode, TemplateID and TemplateSelector", func() {
			dm := defaultMachine()
			dm.Spec.TemplateSource.SourceNode = ""
			dm.Spec.TemplateSource.TemplateID = nil
			dm.Spec.TemplateSelector = nil
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("must define either SourceNode with TemplateID, OR TemplateSelector")))
		})

		It("Should not allow specifying TemplateSelector together with SourceNode and/or TemplateID", func() {
			dm := defaultMachine()
			dm.Spec.TemplateSelector = &TemplateSelector{MatchTags: []string{"test"}}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("must define either SourceNode with TemplateID, OR TemplateSelector")))
		})

		It("Should not allow specifying TemplateSelector with empty MatchTags", func() {
			dm := defaultMachine()
			dm.Spec.TemplateSelector = &TemplateSelector{MatchTags: []string{}}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should have at least 1 items")))
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
				dm.Spec.TemplateSource.SourceNode = ""
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

	Context("AdditionalVolumes format/storage - JSON marshalling", func() {
		It("includes format and storage when set", func() {
			f := TargetFileStorageFormat("qcow2")
			s := "nfs-templates"
			ds := DiskSpec{
				Disk:    "scsi1",
				SizeGB:  80,
				Format:  &f,
				Storage: &s,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi1"`))
			Expect(js).To(ContainSubstring(`"sizeGb":80`))
			Expect(js).To(ContainSubstring(`"format":"qcow2"`))
			Expect(js).To(ContainSubstring(`"storage":"nfs-templates"`))
		})
		It("omits format and storage when nil", func() {
			ds := DiskSpec{
				Disk:    "scsi2",
				SizeGB:  120,
				Format:  nil,
				Storage: nil,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi2"`))
			Expect(js).To(ContainSubstring(`"sizeGb":120`))
			Expect(js).NotTo(ContainSubstring(`"format"`))
			Expect(js).NotTo(ContainSubstring(`"storage"`))
		})
	})

	Context("AdditionalVolumes format/storage - DeepCopy", func() {
		It("preserves per-volume format and storage and performs a deep copy", func() {
			qcow2 := TargetFileStorageFormat("qcow2")
			store := "filestore-a"
			src := &Storage{
				AdditionalVolumes: []DiskSpec{
					{Disk: "scsi1", SizeGB: 80, Format: &qcow2, Storage: &store},
				},
			}
			dst := src.DeepCopy()
			Expect(dst).NotTo(BeNil())
			Expect(dst.AdditionalVolumes).To(HaveLen(1))
			got := dst.AdditionalVolumes[0]
			Expect(got.Disk).To(Equal("scsi1"))
			Expect(got.SizeGB).To(Equal(int32(80)))
			Expect(got.Format).NotTo(BeNil())
			Expect(*got.Format).To(Equal(TargetFileStorageFormat("qcow2")))
			Expect(got.Storage).NotTo(BeNil())
			Expect(*got.Storage).To(Equal("filestore-a"))
			newFmt := TargetFileStorageFormat("raw")
			newStore := "filestore-b"
			*src.AdditionalVolumes[0].Format = newFmt
			*src.AdditionalVolumes[0].Storage = newStore
			Expect(dst.AdditionalVolumes[0].Format).NotTo(BeNil())
			Expect(*dst.AdditionalVolumes[0].Format).To(Equal(TargetFileStorageFormat("qcow2")))
			Expect(dst.AdditionalVolumes[0].Storage).NotTo(BeNil())
			Expect(*dst.AdditionalVolumes[0].Storage).To(Equal("filestore-a"))
		})
	})

	Context("AdditionalVolumes discard - JSON marshalling", func() {
		It("includes discard when explicitly true", func() {
			dTrue := true
			ds := DiskSpec{
				Disk:    "scsi3",
				SizeGB:  60,
				Discard: &dTrue,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi3"`))
			Expect(js).To(ContainSubstring(`"sizeGb":60`))
			Expect(js).To(ContainSubstring(`"discard":true`))
		})
		It("includes discard when explicitly false (non-nil pointer)", func() {
			dFalse := false
			ds := DiskSpec{
				Disk:    "scsi4",
				SizeGB:  70,
				Discard: &dFalse,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi4"`))
			Expect(js).To(ContainSubstring(`"sizeGb":70`))
			// Because Discard is a bool, omitempty does NOT drop a false:
			Expect(js).To(ContainSubstring(`"discard":false`))
		})
		It("omits discard when nil", func() {
			ds := DiskSpec{
				Disk:    "scsi5",
				SizeGB:  80,
				Discard: nil,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi5"`))
			Expect(js).To(ContainSubstring(`"sizeGb":80`))
			Expect(js).NotTo(ContainSubstring(`"discard"`))
		})
	})

	Context("AdditionalVolumes discard - DeepCopy", func() {
		It("preserves per-volume discard and performs a deep copy", func() {
			dTrue := true
			src := &Storage{
				AdditionalVolumes: []DiskSpec{
					{Disk: "scsi6", SizeGB: 90, Discard: &dTrue},
				},
			}
			dst := src.DeepCopy()
			Expect(dst).NotTo(BeNil())
			Expect(dst.AdditionalVolumes).To(HaveLen(1))
			got := dst.AdditionalVolumes[0]
			Expect(got.Disk).To(Equal("scsi6"))
			Expect(got.SizeGB).To(Equal(int32(90)))
			Expect(got.Discard).NotTo(BeNil())
			Expect(*got.Discard).To(BeTrue())
			*src.AdditionalVolumes[0].Discard = false
			Expect(dst.AdditionalVolumes[0].Discard).NotTo(BeNil())
			Expect(*dst.AdditionalVolumes[0].Discard).To(BeTrue())
		})
	})
	Context("AdditionalVolumes iothread - JSON marshalling", func() {
		It("includes iothread when explicitly true", func() {
			tTrue := true
			ds := DiskSpec{
				Disk:     "scsi7",
				SizeGB:   60,
				IOThread: &tTrue,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi7"`))
			Expect(js).To(ContainSubstring(`"sizeGb":60`))
			Expect(js).To(ContainSubstring(`"ioThread":true`))
		})
		It("includes iothread when explicitly false", func() {
			tFalse := false
			ds := DiskSpec{
				Disk:     "scsi8",
				SizeGB:   70,
				IOThread: &tFalse,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi8"`))
			Expect(js).To(ContainSubstring(`"sizeGb":70`))
			Expect(js).To(ContainSubstring(`"ioThread":false`)) // non-nil -> present
		})

		It("omits iothread when nil", func() {
			ds := DiskSpec{
				Disk:     "scsi9",
				SizeGB:   80,
				IOThread: nil,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi9"`))
			Expect(js).To(ContainSubstring(`"sizeGb":80`))
			Expect(js).NotTo(ContainSubstring(`"ioThread"`))
		})
	})

	Context("AdditionalVolumes iothread - DeepCopy", func() {
		It("preserves per-volume iothread and performs a deep copy", func() {
			tTrue := true
			src := &Storage{
				AdditionalVolumes: []DiskSpec{
					{Disk: "scsi10", SizeGB: 90, IOThread: &tTrue},
				},
			}
			dst := src.DeepCopy()
			Expect(dst).NotTo(BeNil())
			Expect(dst.AdditionalVolumes).To(HaveLen(1))
			got := dst.AdditionalVolumes[0]
			Expect(got.Disk).To(Equal("scsi10"))
			Expect(got.SizeGB).To(Equal(int32(90)))
			Expect(got.IOThread).NotTo(BeNil())
			Expect(*got.IOThread).To(BeTrue())
			*src.AdditionalVolumes[0].IOThread = false
			Expect(dst.AdditionalVolumes[0].IOThread).NotTo(BeNil())
			Expect(*dst.AdditionalVolumes[0].IOThread).To(BeTrue())
		})
	})
	Context("AdditionalVolumes ssd - JSON marshalling", func() {
		It("includes ssd when explicitly true", func() {
			sTrue := true
			ds := DiskSpec{
				Disk:   "scsi11",
				SizeGB: 60,
				SSD:    &sTrue,
			}

			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)

			Expect(js).To(ContainSubstring(`"disk":"scsi11"`))
			Expect(js).To(ContainSubstring(`"sizeGb":60`))
			Expect(js).To(ContainSubstring(`"ssd":true`))
		})
		It("includes ssd when explicitly false", func() {
			sFalse := false
			ds := DiskSpec{
				Disk:   "scsi12",
				SizeGB: 70,
				SSD:    &sFalse,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi12"`))
			Expect(js).To(ContainSubstring(`"sizeGb":70`))
			Expect(js).To(ContainSubstring(`"ssd":false`)) // non-nil -> present
		})
		It("omits ssd when nil", func() {
			ds := DiskSpec{
				Disk:   "scsi13",
				SizeGB: 80,
				SSD:    nil,
			}
			b, err := json.Marshal(ds)
			Expect(err).NotTo(HaveOccurred())
			js := string(b)
			Expect(js).To(ContainSubstring(`"disk":"scsi13"`))
			Expect(js).To(ContainSubstring(`"sizeGb":80`))
			Expect(js).NotTo(ContainSubstring(`"ssd"`))
		})
	})

	Context("AdditionalVolumes ssd - DeepCopy", func() {
		It("preserves per-volume ssd and performs a deep copy", func() {
			sTrue := true
			src := &Storage{
				AdditionalVolumes: []DiskSpec{
					{Disk: "scsi14", SizeGB: 90, SSD: &sTrue},
				},
			}
			dst := src.DeepCopy()
			Expect(dst).NotTo(BeNil())
			Expect(dst.AdditionalVolumes).To(HaveLen(1))
			got := dst.AdditionalVolumes[0]
			Expect(got.Disk).To(Equal("scsi14"))
			Expect(got.SizeGB).To(Equal(int32(90)))
			Expect(got.SSD).NotTo(BeNil())
			Expect(*got.SSD).To(BeTrue())
			// Mutate source; destination should remain unchanged
			*src.AdditionalVolumes[0].SSD = false
			Expect(dst.AdditionalVolumes[0].SSD).NotTo(BeNil())
			Expect(*dst.AdditionalVolumes[0].SSD).To(BeTrue())
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
				AdditionalDevices: []AdditionalNetworkDevice{
					{
						NetworkDevice: NetworkDevice{
							IPPoolConfig: IPPoolConfig{
								IPv4PoolRef: &corev1.TypedLocalObjectReference{
									APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
									Kind:     "InClusterIPPool",
									Name:     "some-pool",
								},
							},
						},
						Name:            "net0",
						InterfaceConfig: InterfaceConfig{},
					},
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be at least 1 chars long")))
		})

		It("Should only allow IPAM pool resources in IPv4PoolRef apiGroup", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{
					{
						NetworkDevice: NetworkDevice{
							IPPoolConfig: IPPoolConfig{
								IPv4PoolRef: &corev1.TypedLocalObjectReference{
									APIGroup: ptr.To("apps"),
									Name:     "some-app",
								},
							},
						},
						Name: "net1",
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipv4PoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io")))
		})

		It("Should only allow IPAM pool resources in IPv4PoolRef kind", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{
					{
						NetworkDevice: NetworkDevice{
							IPPoolConfig: IPPoolConfig{
								IPv4PoolRef: &corev1.TypedLocalObjectReference{
									APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
									Kind:     "ConfigMap",
									Name:     "some-app",
								},
							},
						},
						Name:            "net1",
						InterfaceConfig: InterfaceConfig{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipv4PoolRef allows either InClusterIPPool or GlobalInClusterIPPool")))
		})

		It("Should only allow IPAM pool resources in IPv6PoolRef apiGroup", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{
					{
						NetworkDevice: NetworkDevice{
							IPPoolConfig: IPPoolConfig{
								IPv6PoolRef: &corev1.TypedLocalObjectReference{
									APIGroup: ptr.To("apps"),
									Name:     "some-app",
								},
							},
						},
						Name:            "net1",
						InterfaceConfig: InterfaceConfig{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipv6PoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io")))
		})

		It("Should only allow IPAM pool resources in IPv6PoolRef kind", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{
					{
						NetworkDevice: NetworkDevice{
							IPPoolConfig: IPPoolConfig{
								IPv6PoolRef: &corev1.TypedLocalObjectReference{
									APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
									Kind:     "ConfigMap",
									Name:     "some-app",
								},
							},
						},
						Name:            "net1",
						InterfaceConfig: InterfaceConfig{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("ipv6PoolRef allows either InClusterIPPool or GlobalInClusterIPPool")))
		})

		It("Should only allow Machine with additional devices with at least a pool ref", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				AdditionalDevices: []AdditionalNetworkDevice{
					{
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

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("invalid MTU value")))
		})

		It("Should not allow machine with network device mtu greater than 65520", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				Default: &NetworkDevice{
					Bridge: "vmbr0",
					MTU:    ptr.To(uint16(65521)),
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("invalid MTU value")))
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
						Routing: Routing{
							RoutingPolicy: []RoutingPolicySpec{{
								Priority: 32766,
							}},
						},
					}},
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("Cowardly refusing to insert FIB rule matching kernel rules")))
		})

		It("Should not allow machine with network device vlan equal to 0", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				Default: &NetworkDevice{
					Bridge: "vmbr0",
					VLAN:   ptr.To(uint16(0)),
				},
			}

			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("should be greater than or equal to 1")))
		})

		It("Should not allow machine with network device vlan greater than 4094", func() {
			dm := defaultMachine()
			dm.Spec.Network = &NetworkSpec{
				Default: &NetworkDevice{
					Bridge: "vmbr0",
					VLAN:   ptr.To(uint16(4095)),
				},
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
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("spec.vmIDRange.end in body should be greater than or equal to 100")))
		})
		It("Should only allow spec.vmIDRange.end if spec.vmIDRange.start is set", func() {
			dm := defaultMachine()
			dm.Spec.VMIDRange = &VMIDRange{
				End: 100,
			}
			Expect(k8sClient.Create(context.Background(), dm)).Should(MatchError(ContainSubstring("spec.vmIDRange.start in body should be greater than or equal to 100")))
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
