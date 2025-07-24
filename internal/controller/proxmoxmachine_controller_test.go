/*
Copyright 2024 IONOS Cloud.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
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
	})
})
