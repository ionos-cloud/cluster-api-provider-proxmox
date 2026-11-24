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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// newPersistedProxmoxCluster stores a ProxmoxCluster with a credentialsRef in the fake
// client and returns the stored object, so that the scope's patch has something to hit.
func newPersistedProxmoxCluster(t *testing.T, k8sClient ctrlclient.Client) *infrav1.ProxmoxCluster {
	t.Helper()
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
	require.NoError(t, k8sClient.Create(context.Background(), proxmoxCluster))
	return proxmoxCluster
}

func persistedProxmoxAvailable(t *testing.T, k8sClient ctrlclient.Client, proxmoxCluster *infrav1.ProxmoxCluster) *metav1.Condition {
	t.Helper()
	stored := &infrav1.ProxmoxCluster{}
	require.NoError(t, k8sClient.Get(context.Background(), ctrlclient.ObjectKeyFromObject(proxmoxCluster), stored))
	return conditions.Get(stored, infrav1.ProxmoxClusterProxmoxAvailableCondition)
}

func TestNewClusterScope_PersistsCredentialsNotFound(t *testing.T) {
	k8sClient := getFakeClient(t)
	proxmoxCluster := newPersistedProxmoxCluster(t, k8sClient)

	_, err := NewClusterScope(ClusterScopeParams{Client: k8sClient, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: proxmoxCluster, IPAMHelper: &ipam.Helper{}})
	require.ErrorContains(t, err, "failed to get credentials secret")

	cond := persistedProxmoxAvailable(t, k8sClient, proxmoxCluster)
	require.NotNil(t, cond, "the condition must reach the API server, not only the in-memory object")
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, infrav1.ProxmoxClusterProxmoxAvailableCredentialsNotFoundReason, cond.Reason)
}

func TestNewClusterScope_PersistsCredentialsInvalid(t *testing.T) {
	for _, tc := range []struct {
		key, value string
	}{
		{"insecure", "maybe"},
		{"insecure", ""},
		{"endpoint_discovery", "ja"},
		{"root_ca", "not a certificate"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			k8sClient := getFakeClient(t)
			proxmoxCluster := newPersistedProxmoxCluster(t, k8sClient)
			// The fake client does not fold StringData into Data as the API server does.
			creds := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: "default"},
				Data: map[string][]byte{
					"url":   []byte("https://localhost:8006"),
					"token": []byte("test-token"), "secret": []byte("test-secret"),
					tc.key: []byte(tc.value),
				},
			}
			require.NoError(t, k8sClient.Create(context.Background(), &creds))

			_, err := NewClusterScope(ClusterScopeParams{Client: k8sClient, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: proxmoxCluster, IPAMHelper: &ipam.Helper{}})
			require.ErrorContains(t, err, `credentials secret key "`+tc.key+`"`)

			cond := persistedProxmoxAvailable(t, k8sClient, proxmoxCluster)
			require.NotNil(t, cond)
			require.Equal(t, metav1.ConditionFalse, cond.Status)
			require.Equal(t, infrav1.ProxmoxClusterProxmoxAvailableCredentialsInvalidReason, cond.Reason)
			require.Contains(t, cond.Message, `"`+tc.key+`"`)
		})
	}
}

func TestSetupProxmoxClient_SecretBooleans(t *testing.T) {
	// The Secret keys accept the same spellings as PROXMOX_INSECURE; the HTTP client is
	// built but the API is never reached because the URL is unroutable, so only the
	// parse step is under test here. A hanging API must also fail within the timeouts.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	saved := goproxmox.RequestTimeout
	goproxmox.RequestTimeout = 200 * time.Millisecond
	t.Cleanup(func() { goproxmox.RequestTimeout = saved })

	for _, value := range []string{"true", "YES", " on ", "0", "No", "f"} {
		t.Run(value, func(t *testing.T) {
			k8sClient := getFakeClient(t)
			proxmoxCluster := newPersistedProxmoxCluster(t, k8sClient)
			creds := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: "default"},
				Data: map[string][]byte{
					"url": []byte(srv.URL), "token": []byte("test-token"), "secret": []byte("test-secret"),
					"insecure": []byte(value), "endpoint_discovery": []byte(value),
				},
			}
			require.NoError(t, k8sClient.Create(context.Background(), &creds))

			start := time.Now()
			_, err := NewClusterScope(ClusterScopeParams{Client: k8sClient, Cluster: &clusterv1.Cluster{}, ProxmoxCluster: proxmoxCluster, IPAMHelper: &ipam.Helper{}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "invalid boolean value")
			require.Less(t, time.Since(start), 5*time.Second)
			cond := persistedProxmoxAvailable(t, k8sClient, proxmoxCluster)
			if cond != nil {
				require.NotEqual(t, infrav1.ProxmoxClusterProxmoxAvailableCredentialsInvalidReason, cond.Reason)
			}
		})
	}
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

func getFakeClient(t *testing.T) ctrlclient.Client {
	scheme := runtime.NewScheme()

	// Register client-go scheme with the scheme
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	err = clusterv1.AddToScheme(scheme)
	require.NoError(t, err)
	err = infrav1.AddToScheme(scheme)
	require.NoError(t, err)

	return fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&infrav1.ProxmoxCluster{}).Build()
}
