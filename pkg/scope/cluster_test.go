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

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	"github.com/stretchr/testify/require"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
