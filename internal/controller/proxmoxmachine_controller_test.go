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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
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
	})
})

// The condition the controller persists for a ready machine keeps the warning the last
// provisioning step left (cloud-init finished with recoverable errors), and only that: the
// message of any other state is not carried into a provisioned machine.
func TestProvisionedCondition(t *testing.T) {
	const warning = "cloud-init finished with recoverable errors (exit code 2): status: done"

	for _, tc := range []struct {
		name string
		prev *metav1.Condition
		want string
	}{
		{name: "no previous condition", prev: nil, want: ""},
		{name: "bootstrap ready without a warning", prev: &metav1.Condition{Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapReadyReason}, want: ""},
		{name: "bootstrap ready with a cloud-init warning", prev: &metav1.Condition{Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapReadyReason, Message: warning}, want: warning},
		{name: "already provisioned keeps the warning", prev: &metav1.Condition{Status: metav1.ConditionTrue, Reason: clusterv1.ProvisionedReason, Message: warning}, want: warning},
		{name: "another state's message is not carried over", prev: &metav1.Condition{Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason, Message: "vmid collision"}, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := &infrav1.ProxmoxMachine{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"}}
			if tc.prev != nil {
				tc.prev.Type = infrav1.ProxmoxMachineVirtualMachineProvisionedCondition
				if tc.prev.Status == "" {
					tc.prev.Status = metav1.ConditionFalse
				}
				conditions.Set(machine, *tc.prev)
			}

			conditions.Set(machine, provisionedCondition(machine))

			got := conditions.Get(machine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
			require.NotNil(t, got)
			require.Equal(t, metav1.ConditionTrue, got.Status)
			require.Equal(t, clusterv1.ProvisionedReason, got.Reason)
			require.Equal(t, tc.want, got.Message)
		})
	}
}
