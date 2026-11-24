/*
Copyright 2026 IONOS Cloud.

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
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
)

// fakeProxmox answers the version call NewAPIClient makes and tracks its connections.
type fakeProxmox struct {
	*httptest.Server
	versionCalls atomic.Int32

	mu   sync.Mutex
	open map[net.Conn]bool
}

func newFakeProxmox(t *testing.T) *fakeProxmox {
	t.Helper()
	f := &fakeProxmox{open: map[net.Conn]bool{}}
	f.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/version" {
			f.versionCalls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"release":"8.4","version":"8.4.1","repoid":"test"}}`))
	}))
	f.Config.ConnState = func(c net.Conn, state http.ConnState) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch state {
		case http.StateNew:
			f.open[c] = true
		case http.StateClosed, http.StateHijacked:
			delete(f.open, c)
		default:
		}
	}
	f.StartTLS()
	t.Cleanup(f.Close)
	return f
}

func (f *fakeProxmox) openConns() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.open)
}

func TestNewClusterScope_ReusesProxmoxClientPerCredentialsSecret(t *testing.T) {
	previous := credentialsClients
	credentialsClients = newClientCache()
	t.Cleanup(func() { credentialsClients = previous })

	server := newFakeProxmox(t)
	k8sClient := getFakeClient(t)
	ctx := context.Background()

	creds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "proxmox-credentials", Namespace: "default"},
		Data: map[string][]byte{
			"url":    []byte(server.URL),
			"token":  []byte("capmox@pve!capi"),
			"secret": []byte("test-secret"),
		},
	}
	require.NoError(t, k8sClient.Create(ctx, creds))

	proxmoxCluster := &infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "proxmoxcluster", Namespace: "default"},
		Spec: infrav1.ProxmoxClusterSpec{
			AllowedNodes:   []string{"pve"},
			CredentialsRef: &corev1.SecretReference{Name: creds.Name, Namespace: creds.Namespace},
		},
	}
	params := ClusterScopeParams{
		Client:         k8sClient,
		Cluster:        &clusterv1.Cluster{},
		ProxmoxCluster: proxmoxCluster,
		IPAMHelper:     &ipam.Helper{},
	}

	first, err := NewClusterScope(params)
	require.NoError(t, err)
	second, err := NewClusterScope(params)
	require.NoError(t, err)

	require.Same(t, first.ProxmoxClient, second.ProxmoxClient, "a reconcile with unchanged credentials reuses the client")
	require.Equal(t, int32(1), server.versionCalls.Load())
	require.Equal(t, 1, server.openConns())

	creds.Data["secret"] = []byte("rotated-secret")
	require.NoError(t, k8sClient.Update(ctx, creds))

	third, err := NewClusterScope(params)
	require.NoError(t, err)

	require.NotSame(t, first.ProxmoxClient, third.ProxmoxClient, "changed credentials build a new client")
	require.Equal(t, int32(2), server.versionCalls.Load())
	require.Eventually(t, func() bool { return server.openConns() == 1 }, 5*time.Second, 10*time.Millisecond,
		"the replaced client's idle connection is closed")
}
