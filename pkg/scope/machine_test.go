/*
Copyright 2023 IONOS Cloud.

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
	"testing"

	"github.com/stretchr/testify/require"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewMachineScope(test.params)
			require.Error(t, err)
		})
	}
}
