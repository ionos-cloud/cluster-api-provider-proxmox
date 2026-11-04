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

package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func TestReconcileFailureDomains(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1.AddToScheme(scheme))
	require.NoError(t, infrav1.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	newClusterScope := func(azs []infrav1.AvailabilityZoneSpec) *scope.ClusterScope {
		proxmoxCluster := &infrav1.ProxmoxCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: metav1.NamespaceDefault},
			Spec:       infrav1.ProxmoxClusterSpec{AvailabilityZones: azs},
		}
		clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
			Client:         kubeClient,
			Cluster:        &clusterv1.Cluster{},
			ProxmoxCluster: proxmoxCluster,
			ProxmoxClient:  proxmoxtest.NewMockClient(t),
			IPAMHelper:     &ipam.Helper{},
		})
		require.NoError(t, err)
		return clusterScope
	}

	t.Run("syncs status from spec availability zones", func(t *testing.T) {
		clusterScope := newClusterScope([]infrav1.AvailabilityZoneSpec{
			{Name: "az-1", Nodes: []string{"pve1"}},
			{Name: "az-2", Nodes: []string{"pve2"}},
		})

		reconcileFailureDomains(clusterScope)

		fds := clusterScope.ProxmoxCluster.Status.FailureDomains
		require.Len(t, fds, 2)
		require.Equal(t, "az-1", fds[0].Name)
		require.Equal(t, "az-2", fds[1].Name)
		require.True(t, *fds[0].ControlPlane)
		require.True(t, *fds[1].ControlPlane)
	})

	t.Run("clears status when no availability zones are configured", func(t *testing.T) {
		clusterScope := newClusterScope(nil)
		clusterScope.ProxmoxCluster.Status.FailureDomains = []clusterv1.FailureDomain{
			{Name: "stale", ControlPlane: new(true)},
		}

		reconcileFailureDomains(clusterScope)

		require.Empty(t, clusterScope.ProxmoxCluster.Status.FailureDomains)
	})
}
