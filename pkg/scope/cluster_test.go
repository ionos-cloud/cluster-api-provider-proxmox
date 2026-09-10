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

package scope

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
)

func TestNewClusterScope_MissingParams(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()

	tests := []struct {
		name   string
		params ClusterScopeParams
	}{
		{"missing client", ClusterScopeParams{Cluster: &clusterv1.Cluster{}, ProxmoxCluster: &infrav1.ProxmoxCluster{}, ProxmoxClient: &goproxmox.APIClient{}, IPAMHelper: &ipam.Helper{}}},
		{"missing cluster", ClusterScopeParams{Client: k8sClient, ProxmoxCluster: &infrav1.ProxmoxCluster{}, ProxmoxClient: &goproxmox.APIClient{}, IPAMHelper: &ipam.Helper{}}},
		{"missing proxmox cluster", ClusterScopeParams{Client: k8sClient, Cluster: &clusterv1.Cluster{}, ProxmoxClient: &goproxmox.APIClient{}, IPAMHelper: &ipam.Helper{}}},
		{"missing ipam helper", ClusterScopeParams{Client: k8sClient, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: &infrav1.ProxmoxCluster{}, ProxmoxClient: &goproxmox.APIClient{}}},
		{"missing proxmox client", ClusterScopeParams{Client: k8sClient, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: &infrav1.ProxmoxCluster{}, IPAMHelper: &ipam.Helper{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewClusterScope(test.params)
			require.Error(t, err)
		})
	}
}

func TestNewClusterScope_MissingProxmoxClient(t *testing.T) {
	k8sClient := getFakeClient(t)

	proxmoxCluster := &infrav1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: infrav1.GroupVersion.String(),
			Kind:       "ProxmoxCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
		Spec: infrav1.ProxmoxClusterSpec{
			AllowedNodes: []string{"pve", "pve-2"},
		},
	}

	tests := []struct {
		name   string
		params ClusterScopeParams
	}{
		{"missing proxmox client in ref", ClusterScopeParams{Client: k8sClient, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: proxmoxCluster, IPAMHelper: &ipam.Helper{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewClusterScope(test.params)
			require.Error(t, err)
			cond := conditions.Get(proxmoxCluster, infrav1.ProxmoxClusterProxmoxAvailableCondition)
			require.NotNil(t, cond)
			require.Equal(t, metav1.ConditionFalse, cond.Status)
		})
	}
}

func TestNewClusterScope_NoDefaultCredentialsNeededWhenZonesCoverAllNodes(t *testing.T) {
	// Regression test: a ProxmoxCluster spanning multiple physically separate Proxmox VE
	// clusters (one per availability zone, each with its own credentialsRef) must not fail
	// just because it lacks a top-level spec.credentialsRef / global controller credentials,
	// as long as every allowed node is covered by a zone with its own credentialsRef.
	k8sClient := getFakeClient(t)

	proxmoxCluster := &infrav1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: infrav1.GroupVersion.String(),
			Kind:       "ProxmoxCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
		Spec: infrav1.ProxmoxClusterSpec{
			AllowedNodes: []string{"skbo001004", "skbo002004"},
			AvailabilityZones: []infrav1.AvailabilityZoneSpec{
				{Name: "az1", Nodes: []string{"skbo001004"}, CredentialsRef: &corev1.SecretReference{Name: "proxmox-az1-credentials"}},
				{Name: "az2", Nodes: []string{"skbo002004"}, CredentialsRef: &corev1.SecretReference{Name: "proxmox-az2-credentials"}},
			},
		},
	}

	clusterScope, err := NewClusterScope(ClusterScopeParams{
		Client:         k8sClient,
		Cluster:        &clusterv1.Cluster{},
		ProxmoxCluster: proxmoxCluster,
		IPAMHelper:     &ipam.Helper{},
	})
	require.NoError(t, err)
	require.Nil(t, clusterScope.ProxmoxClient)

	// The default client is never expected to be used here, but requesting it explicitly
	// (e.g. for zone "") must fail clearly instead of returning a nil client.
	_, err = clusterScope.GetProxmoxClient(context.Background(), "")
	require.Error(t, err)
}

func TestNewClusterScope_SetupProxmoxClient(t *testing.T) {
	k8sClient := getFakeClient(t)

	proxmoxCluster := &infrav1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: infrav1.GroupVersion.String(),
			Kind:       "ProxmoxCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
		Spec: infrav1.ProxmoxClusterSpec{
			AllowedNodes: []string{"pve", "pve-2"},
			CredentialsRef: &corev1.SecretReference{
				Name:      "test-secret",
				Namespace: "default",
			},
		},
	}

	params := ClusterScopeParams{Client: k8sClient, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: proxmoxCluster, IPAMHelper: &ipam.Helper{}}
	_, err := NewClusterScope(params)
	require.Error(t, err)

	creds := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		StringData: map[string]string{
			"url":    "https://localhost:8006",
			"token":  "test-token",
			"secret": "test-secret",
		},
	}

	err = k8sClient.Create(context.Background(), &creds)
	require.NoError(t, err)

	_, err = NewClusterScope(params)
	require.Error(t, err)
}

func TestListProxmoxMachinesForCluster(t *testing.T) {
	k8sClient := getFakeClient(t)
	proxmoxClient := proxmoxtest.NewMockClient(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
	}
	err := k8sClient.Create(context.Background(), cluster)
	require.NoError(t, err)

	proxmoxCluster := &infrav1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: infrav1.GroupVersion.String(),
			Kind:       "ProxmoxCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
		Spec: infrav1.ProxmoxClusterSpec{
			AllowedNodes: []string{"pve", "pve-2"},
			CredentialsRef: &corev1.SecretReference{
				Name:      "test-secret",
				Namespace: "default",
			},
		},
	}
	err = k8sClient.Create(context.Background(), proxmoxCluster)
	require.NoError(t, err)

	creds := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		StringData: map[string]string{
			"url":    "https://localhost:8006",
			"token":  "test-token",
			"secret": "test-secret",
		},
	}

	err = k8sClient.Create(context.Background(), &creds)
	require.NoError(t, err)

	params := ClusterScopeParams{Client: k8sClient, Cluster: cluster, ProxmoxCluster: proxmoxCluster, ProxmoxClient: proxmoxClient, IPAMHelper: &ipam.Helper{}}
	clusterScope, err := NewClusterScope(params)
	require.NoError(t, err)

	expectedMachineList := &infrav1.ProxmoxMachineList{
		Items: []infrav1.ProxmoxMachine{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine01",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: clusterScope.Name(),
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine02",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: clusterScope.Name(),
					},
				},
			},
		},
	}

	for machineIdx := range expectedMachineList.Items {
		err = k8sClient.Create(context.Background(), &expectedMachineList.Items[machineIdx])
		require.NoError(t, err)
		// As the k8sClient sets ResourceVersion to 1, we also set it in the expectedMachineList.
		expectedMachineList.Items[machineIdx].ResourceVersion = "1"
	}

	unexpectedMachine := &infrav1.ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-cluster-machine01",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "other-cluster",
			},
		},
	}
	err = k8sClient.Create(context.Background(), unexpectedMachine)
	require.NoError(t, err)

	machines, err := clusterScope.ListProxmoxMachinesForCluster(context.Background())

	require.NoError(t, err)
	require.Equal(t, expectedMachineList.Items, machines)
}

func TestClusterScope_GetProxmoxClient(t *testing.T) {
	k8sClient := getFakeClient(t)
	defaultClient := proxmoxtest.NewMockClient(t)

	proxmoxCluster := &infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
		Spec: infrav1.ProxmoxClusterSpec{
			AvailabilityZones: []infrav1.AvailabilityZoneSpec{
				{Name: "az-1", Nodes: []string{"pve1"}},
				{Name: "az-2", Nodes: []string{"pve2"}, CredentialsRef: &corev1.SecretReference{Name: "az-2-secret", Namespace: "default"}},
			},
		},
	}

	err := k8sClient.Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "az-2-secret", Namespace: "default"},
		StringData: map[string]string{"url": "https://az2.example.com:8006", "token": "token", "secret": "secret"},
	})
	require.NoError(t, err)

	clusterScope, err := NewClusterScope(ClusterScopeParams{
		Client:         k8sClient,
		Cluster:        &clusterv1.Cluster{},
		ProxmoxCluster: proxmoxCluster,
		ProxmoxClient:  defaultClient,
		IPAMHelper:     &ipam.Helper{},
	})
	require.NoError(t, err)

	// No zone falls back to the default client.
	client, err := clusterScope.GetProxmoxClient(context.Background(), "")
	require.NoError(t, err)
	require.Same(t, defaultClient, client)

	// A zone without its own credentialsRef falls back to the default client.
	client, err = clusterScope.GetProxmoxClient(context.Background(), "az-1")
	require.NoError(t, err)
	require.Same(t, defaultClient, client)

	// A zone with its own credentialsRef resolves its own secret and attempts to build a
	// dedicated client (fails here since there's no real Proxmox API to reach in this test).
	_, err = clusterScope.GetProxmoxClient(context.Background(), "az-2")
	require.Error(t, err)

	// An unknown zone falls back to the default client.
	client, err = clusterScope.GetProxmoxClient(context.Background(), "unknown")
	require.NoError(t, err)
	require.Same(t, defaultClient, client)
}

func TestClusterScope_GetProxmoxClientForNode(t *testing.T) {
	k8sClient := getFakeClient(t)
	defaultClient := proxmoxtest.NewMockClient(t)

	proxmoxCluster := &infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
		Spec: infrav1.ProxmoxClusterSpec{
			AvailabilityZones: []infrav1.AvailabilityZoneSpec{
				{Name: "az-1", Nodes: []string{"pve1"}},
				{Name: "az-2", Nodes: []string{"pve2"}, CredentialsRef: &corev1.SecretReference{Name: "az-2-secret", Namespace: "default"}},
			},
		},
	}

	err := k8sClient.Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "az-2-secret", Namespace: "default"},
		StringData: map[string]string{"url": "https://az2.example.com:8006", "token": "token", "secret": "secret"},
	})
	require.NoError(t, err)

	clusterScope, err := NewClusterScope(ClusterScopeParams{
		Client:         k8sClient,
		Cluster:        &clusterv1.Cluster{},
		ProxmoxCluster: proxmoxCluster,
		ProxmoxClient:  defaultClient,
		IPAMHelper:     &ipam.Helper{},
	})
	require.NoError(t, err)

	// A node in a zone without its own credentialsRef falls back to the default client.
	client, err := clusterScope.GetProxmoxClientForNode(context.Background(), "pve1")
	require.NoError(t, err)
	require.Same(t, defaultClient, client)

	// A node not listed in any availability zone falls back to the default client.
	client, err = clusterScope.GetProxmoxClientForNode(context.Background(), "pve-unknown")
	require.NoError(t, err)
	require.Same(t, defaultClient, client)

	// A node in a zone with its own credentialsRef resolves that zone's client.
	_, err = clusterScope.GetProxmoxClientForNode(context.Background(), "pve2")
	require.Error(t, err)
}

func getFakeClient(t *testing.T) ctrlclient.Client {
	scheme := runtime.NewScheme()

	// Register client-go scheme with the scheme
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	err = clusterv1.AddToScheme(scheme)
	require.NoError(t, err)
	err = infrav1.AddToScheme(scheme)
	require.NoError(t, err)

	return fake.NewClientBuilder().WithScheme(scheme).Build()
}
