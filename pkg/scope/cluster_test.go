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

package scope

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clustererrors "sigs.k8s.io/cluster-api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
)

func TestNewClusterScope_MissingParams(t *testing.T) {
	client := fake.NewClientBuilder().Build()

	tests := []struct {
		name   string
		params ClusterScopeParams
	}{
		{"missing client", ClusterScopeParams{Cluster: &clusterv1.Cluster{}, ProxmoxCluster: &infrav1alpha1.ProxmoxCluster{}, ProxmoxClient: &goproxmox.APIClient{}, IPAMHelper: &ipam.Helper{}}},
		{"missing cluster", ClusterScopeParams{Client: client, ProxmoxCluster: &infrav1alpha1.ProxmoxCluster{}, ProxmoxClient: &goproxmox.APIClient{}, IPAMHelper: &ipam.Helper{}}},
		{"missing proxmox cluster", ClusterScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, ProxmoxClient: &goproxmox.APIClient{}, IPAMHelper: &ipam.Helper{}}},
		{"missing ipam helper", ClusterScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: &infrav1alpha1.ProxmoxCluster{}, ProxmoxClient: &goproxmox.APIClient{}}},
		{"missing proxmox client", ClusterScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: &infrav1alpha1.ProxmoxCluster{}, IPAMHelper: &ipam.Helper{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewClusterScope(test.params)
			require.Error(t, err)
		})
	}
}

func TestNewClusterScope_MissingProxmoxClient(t *testing.T) {
	client := getFakeClient(t)

	proxmoxCluster := &infrav1alpha1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: infrav1alpha1.GroupVersion.String(),
			Kind:       "ProxmoxCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
		Spec: infrav1alpha1.ProxmoxClusterSpec{
			AllowedNodes: []string{"pve", "pve-2"},
		},
	}

	tests := []struct {
		name   string
		params ClusterScopeParams
	}{
		{"missing proxmox client in ref", ClusterScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: proxmoxCluster, IPAMHelper: &ipam.Helper{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewClusterScope(test.params)
			require.Error(t, err)
			require.Equal(t, proxmoxCluster.Status.FailureReason, ptr.To(clustererrors.InvalidConfigurationClusterError))
		})
	}
}

func TestNewClusterScope_SetupProxmoxClient(t *testing.T) {
	client := getFakeClient(t)

	proxmoxCluster := &infrav1alpha1.ProxmoxCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: infrav1alpha1.GroupVersion.String(),
			Kind:       "ProxmoxCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxmoxcluster",
			Namespace: "default",
		},
		Spec: infrav1alpha1.ProxmoxClusterSpec{
			AllowedNodes: []string{"pve", "pve-2"},
			CredentialsRef: &corev1.SecretReference{
				Name:      "test-secret",
				Namespace: "default",
			},
		},
	}

	params := ClusterScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: proxmoxCluster, IPAMHelper: &ipam.Helper{}}
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

	err = client.Create(context.Background(), &creds)
	require.NoError(t, err)

	_, err = NewClusterScope(params)
	require.Error(t, err)
}

func getFakeClient(t *testing.T) ctrlclient.Client {
	scheme := runtime.NewScheme()

	// Register client-go scheme with the scheme
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	err = clusterv1.AddToScheme(scheme)
	require.NoError(t, err)
	err = infrav1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	return fake.NewClientBuilder().WithScheme(scheme).Build()
}
