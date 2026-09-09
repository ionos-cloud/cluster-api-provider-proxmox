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

	"github.com/stretchr/testify/require"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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

func TestMachineScope_ResolvePlacement(t *testing.T) {
	cluster := &infrav1.ProxmoxCluster{
		Spec: infrav1.ProxmoxClusterSpec{
			AllowedNodes: []string{"cluster-node1", "cluster-node2"},
			ZoneConfigs: []infrav1.ZoneConfigSpec{
				{
					Zone:  new("zone-a"),
					Nodes: []string{"pve1", "pve2"},
				},
				{
					Zone: new("zone-b"),
					// No explicit nodes.
				},
			},
		},
	}

	newScope := func(failureDomain string, proxmoxMachine *infrav1.ProxmoxMachine) *MachineScope {
		return &MachineScope{
			Machine: &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{FailureDomain: failureDomain},
			},
			InfraCluster:   &ClusterScope{ProxmoxCluster: cluster},
			ProxmoxMachine: proxmoxMachine,
		}
	}

	t.Run("machine spec nodes take precedence over cluster spec", func(t *testing.T) {
		scope := newScope("", &infrav1.ProxmoxMachine{
			Spec: infrav1.ProxmoxMachineSpec{AllowedNodes: []string{"spec-node1"}},
		})
		require.NoError(t, scope.resolvePlacement())
		require.Equal(t, []string{"spec-node1"}, scope.AllowedNodes())
		require.Nil(t, scope.Zone())
	})

	t.Run("cluster spec fallback when machine spec is empty", func(t *testing.T) {
		scope := newScope("", &infrav1.ProxmoxMachine{})
		require.NoError(t, scope.resolvePlacement())
		require.Equal(t, []string{"cluster-node1", "cluster-node2"}, scope.AllowedNodes())
	})

	t.Run("network zone resolved without failure domain", func(t *testing.T) {
		scope := newScope("", &infrav1.ProxmoxMachine{
			Spec: infrav1.ProxmoxMachineSpec{
				Network: &infrav1.NetworkSpec{Zone: new("net-zone")},
			},
		})
		require.NoError(t, scope.resolvePlacement())
		require.Equal(t, "net-zone", *scope.Zone())
	})

	t.Run("failure domain nodes and zone win over specs", func(t *testing.T) {
		scope := newScope("zone-a", &infrav1.ProxmoxMachine{
			Spec: infrav1.ProxmoxMachineSpec{
				AllowedNodes: []string{"spec-node1"},
				Network:      &infrav1.NetworkSpec{Zone: new("net-zone")},
			},
		})
		require.NoError(t, scope.resolvePlacement())
		require.Equal(t, []string{"pve1", "pve2"}, scope.AllowedNodes())
		require.Equal(t, "zone-a", *scope.Zone())
	})

	t.Run("zone without nodes falls back to specs", func(t *testing.T) {
		scope := newScope("zone-b", &infrav1.ProxmoxMachine{
			Spec: infrav1.ProxmoxMachineSpec{AllowedNodes: []string{"spec-node1"}},
		})
		require.NoError(t, scope.resolvePlacement())
		require.Equal(t, []string{"spec-node1"}, scope.AllowedNodes())
		require.Equal(t, "zone-b", *scope.Zone())
	})

	t.Run("zone not found returns typed error, fallbacks stay usable", func(t *testing.T) {
		scope := newScope("zone-c", &infrav1.ProxmoxMachine{})
		err := scope.resolvePlacement()
		require.Error(t, err)
		require.ErrorAs(t, err, &FailureDomainNotFoundError{})
		require.Contains(t, err.Error(), "zone-c")
		// The scope must remain usable for paths that proceed anyway (deletion).
		require.Equal(t, []string{"cluster-node1", "cluster-node2"}, scope.AllowedNodes())
	})
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
