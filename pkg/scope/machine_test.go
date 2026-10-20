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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/stretchr/testify/require"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
)

func TestNewMachineScope_MissingParams(t *testing.T) {
	client := fake.NewClientBuilder().Build()

	tests := []struct {
		name   string
		params MachineScopeParams
	}{
		{"missing client", MachineScopeParams{Cluster: &clusterv1.Cluster{}, Machine: &clusterv1.Machine{}, InfraCluster: &ClusterScope{}, ProxmoxMachine: &infrav1.ProxmoxMachine{}, IPAMHelper: &ipam.Helper{}}},
		{"missing machine", MachineScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, InfraCluster: &ClusterScope{}, ProxmoxMachine: &infrav1.ProxmoxMachine{}, IPAMHelper: &ipam.Helper{}}},
		{"missing cluster", MachineScopeParams{Client: client, Machine: &clusterv1.Machine{}, InfraCluster: &ClusterScope{}, ProxmoxMachine: &infrav1.ProxmoxMachine{}, IPAMHelper: &ipam.Helper{}}},
		{"missing proxmox machine", MachineScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, Machine: &clusterv1.Machine{}, InfraCluster: &ClusterScope{}, IPAMHelper: &ipam.Helper{}}},
		{"missing proxmox cluster", MachineScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, Machine: &clusterv1.Machine{}, ProxmoxMachine: &infrav1.ProxmoxMachine{}, IPAMHelper: &ipam.Helper{}}},
		{"missing ipam helper", MachineScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, Machine: &clusterv1.Machine{}, InfraCluster: &ClusterScope{}, ProxmoxMachine: &infrav1.ProxmoxMachine{}}},
		{"missing scheme", MachineScopeParams{Client: client, Cluster: &clusterv1.Cluster{}, Machine: &clusterv1.Machine{}, InfraCluster: &ClusterScope{}, ProxmoxMachine: &infrav1.ProxmoxMachine{}, IPAMHelper: &ipam.Helper{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewMachineScope(test.params)
			require.Error(t, err)
		})
	}
}

func TestMachineScope_Role(t *testing.T) {
	m := clusterv1.Machine{}
	scope := MachineScope{
		Machine: &m,
	}

	require.Equal(t, scope.Role(), "node")

	m.SetLabels(map[string]string{clusterv1.MachineControlPlaneLabel: "kcp"})
	require.Equal(t, scope.Role(), "control-plane")
}

func TestMachineScope_GetProviderID(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	require.Empty(t, scope.GetProviderID())

	scope.SetProviderID("6b08012f-f589-4c3f-bffa-cf9fa8b29e02")
	require.Equal(t, scope.GetProviderID(), "proxmox://6b08012f-f589-4c3f-bffa-cf9fa8b29e02")
}

func TestMachineScope_GetVirtualMachineID(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	require.Equal(t, scope.GetVirtualMachineID(), int64(-1))

	scope.SetVirtualMachineID(100)
	require.Equal(t, scope.GetVirtualMachineID(), int64(100))
}

func TestMachineScope_VirtualMachineCloneStarted(t *testing.T) {
	tests := []struct {
		name    string
		vmID    *int64
		taskRef *string
		started bool
	}{
		{"nothing started", nil, nil, false},
		{"vm id set", new(int64(100)), nil, true},
		{"clone task set", nil, new("UPID:pve:00000001:00000001:00000001:qmclone:100:root@pam:"), true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stored := &infrav1.ProxmoxMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: metav1.NamespaceDefault},
				Spec:       infrav1.ProxmoxMachineSpec{VirtualMachineID: test.vmID},
				Status:     infrav1.ProxmoxMachineStatus{TaskRef: test.taskRef},
			}
			scope := newCloneStartedScope(t, stored)

			started, err := scope.VirtualMachineCloneStarted(context.Background())
			require.NoError(t, err)
			require.Equal(t, test.started, started)
		})
	}
}

func TestMachineScope_VirtualMachineCloneStarted_NotFound(t *testing.T) {
	scope := newCloneStartedScope(t)

	_, err := scope.VirtualMachineCloneStarted(context.Background())
	require.Error(t, err)
}

// newCloneStartedScope returns a scope whose ProxmoxMachine has nothing set, as a stale cache
// would return it, and whose client holds the stored objects.
func newCloneStartedScope(t *testing.T, stored ...client.Object) *MachineScope {
	scheme := runtime.NewScheme()
	require.NoError(t, infrav1.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored...).Build()

	scope, err := NewMachineScope(MachineScopeParams{
		Client:         kubeClient,
		Cluster:        &clusterv1.Cluster{},
		Machine:        &clusterv1.Machine{},
		InfraCluster:   &ClusterScope{},
		ProxmoxMachine: &infrav1.ProxmoxMachine{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: metav1.NamespaceDefault}},
		IPAMHelper:     &ipam.Helper{},
	})
	require.NoError(t, err)
	return scope
}

func TestMachineScope_SetReady(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	scope.SetReady()
	require.True(t, *scope.ProxmoxMachine.Status.Initialization.Provisioned)

	scope.SetNotReady()
	require.False(t, *scope.ProxmoxMachine.Status.Initialization.Provisioned)
}

func TestMachineScope_HasFailed(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	require.False(t, scope.HasFailed())
}

func TestMachineScope_SkipQemuCheckEnabled(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			Checks: &infrav1.ProxmoxMachineChecks{
				SkipCloudInitStatus: new(true),
			},
		},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	require.True(t, scope.SkipCloudInitCheck())
}

func TestMachineScope_SkipQemuCheck(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	require.False(t, scope.SkipCloudInitCheck())
}

func TestMachineScope_SkipCloudInitCheckEnabled(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			Checks: &infrav1.ProxmoxMachineChecks{
				SkipCloudInitStatus: new(true),
			},
		},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	require.True(t, scope.SkipCloudInitCheck())
}

func TestMachineScope_SkipCloudInit(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	require.False(t, scope.SkipQemuGuestCheck())
}

func TestMachineScope_SkipQemuDisablesCloudInitCheck(t *testing.T) {
	p := infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			Checks: &infrav1.ProxmoxMachineChecks{
				SkipQemuGuestAgent: new(true),
			},
		},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
	}

	require.True(t, scope.SkipCloudInitCheck())
}

func TestMachineScope_GetBootstrapSecret(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	p := infrav1.ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
		Spec:       infrav1.ProxmoxMachineSpec{},
	}
	m := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: new("bootstrap"),
			},
		},
		Status: clusterv1.MachineStatus{},
	}
	scope := MachineScope{
		ProxmoxMachine: &p,
		client:         client,
		Machine:        &m,
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap",
			Namespace: "test",
		},
	}
	require.NoErrorf(t, client.Create(context.Background(), &secret), "")

	bootstrapSecret := corev1.Secret{}
	require.NoErrorf(t, scope.GetBootstrapSecret(context.Background(), &bootstrapSecret), "")
	require.Equal(t, secret.GetName(), bootstrapSecret.GetName())
}
