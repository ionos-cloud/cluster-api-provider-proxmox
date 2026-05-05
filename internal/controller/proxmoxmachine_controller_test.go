/*
Copyright 2024-2026 IONOS Cloud.

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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

var _ = Describe("ProxmoxMachineReconciler", func() {
	BeforeEach(func() {})
	AfterEach(func() {})

	Context("Reconcile an ProxmoxMachine", func() {
		It("should not error with minimal set up", func() {
			reconciler := &ProxmoxMachineReconciler{
				Client:        k8sClient,
				Scheme:        runtime.NewScheme(),
				ProxmoxClient: proxmoxClient,
			}
			By("Calling reconcile")
			ctx := context.Background()
			instance := &infrav1.ProxmoxMachine{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"}}
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: instance.Namespace,
					Name:      instance.Name,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})

		It("should remove the finalizer when the owner Machine is missing during deletion", func() {
			reconciler := &ProxmoxMachineReconciler{
				Client:        k8sClient,
				Scheme:        runtime.NewScheme(),
				ProxmoxClient: proxmoxClient,
			}
			ctx := context.Background()

			By("Creating a ProxmoxMachine with a Machine OwnerRef pointing to a non-existent Machine, and our finalizer set")
			instance := &infrav1.ProxmoxMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "orphan-pmm",
					Namespace:  "default",
					Finalizers: []string{infrav1.MachineFinalizer},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Machine",
						Name:       "missing-machine",
						UID:        "11111111-1111-1111-1111-111111111111",
					}},
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
						BootVolume: &infrav1.DiskSize{Disk: "scsi[0]", SizeGB: 10},
					},
					Network: &infrav1.NetworkSpec{
						NetworkDevices: []infrav1.NetworkDevice{{
							Name:   infrav1.NetName("net0"),
							Bridge: ptr.To("vmbr1"),
							Model:  ptr.To("virtio"),
							MTU:    ptr.To(int32(1500)),
						}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			By("Issuing a Delete so the API server sets a deletionTimestamp (finalizer keeps the object alive)")
			Expect(k8sClient.Delete(ctx, instance)).To(Succeed())

			By("Reconciling — owner Machine NotFound on the delete path should remove the finalizer instead of erroring")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			By("Verifying the ProxmoxMachine has been reaped by the API server now that the finalizer is gone")
			got := &infrav1.ProxmoxMachine{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, got)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected ProxmoxMachine to be gone after finalizer removal")
		})
	})
})
