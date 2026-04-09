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

package convert

import "testing"

func TestDetectResource(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantID   ResourceID
		wantType ConverterType
	}{
		{
			name:     "ProxmoxCluster v1alpha1",
			yaml:     "apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1\nkind: ProxmoxCluster",
			wantID:   ResourceID{APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1", Kind: "ProxmoxCluster"},
			wantType: ConverterCAPMOX,
		},
		{
			name:     "ProxmoxMachine v1alpha1",
			yaml:     "apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1\nkind: ProxmoxMachine",
			wantID:   ResourceID{APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1", Kind: "ProxmoxMachine"},
			wantType: ConverterCAPMOX,
		},
		{
			name:     "ProxmoxMachineTemplate v1alpha1",
			yaml:     "apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1\nkind: ProxmoxMachineTemplate",
			wantID:   ResourceID{APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1", Kind: "ProxmoxMachineTemplate"},
			wantType: ConverterCAPMOX,
		},
		{
			name:     "ProxmoxClusterTemplate v1alpha1",
			yaml:     "apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1\nkind: ProxmoxClusterTemplate",
			wantID:   ResourceID{APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1", Kind: "ProxmoxClusterTemplate"},
			wantType: ConverterCAPMOX,
		},
		{
			name:     "Cluster v1beta1",
			yaml:     "apiVersion: cluster.x-k8s.io/v1beta1\nkind: Cluster",
			wantID:   ResourceID{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Cluster"},
			wantType: ConverterCAPI,
		},
		{
			name:     "MachineDeployment v1beta1",
			yaml:     "apiVersion: cluster.x-k8s.io/v1beta1\nkind: MachineDeployment",
			wantID:   ResourceID{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "MachineDeployment"},
			wantType: ConverterCAPI,
		},
		{
			name:     "KubeadmControlPlane v1beta1",
			yaml:     "apiVersion: controlplane.cluster.x-k8s.io/v1beta1\nkind: KubeadmControlPlane",
			wantID:   ResourceID{APIVersion: "controlplane.cluster.x-k8s.io/v1beta1", Kind: "KubeadmControlPlane"},
			wantType: ConverterCAPI,
		},
		{
			name:     "KubeadmConfigTemplate v1beta1",
			yaml:     "apiVersion: bootstrap.cluster.x-k8s.io/v1beta1\nkind: KubeadmConfigTemplate",
			wantID:   ResourceID{APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1", Kind: "KubeadmConfigTemplate"},
			wantType: ConverterCAPI,
		},
		{
			name:     "ConfigMap passthrough",
			yaml:     "apiVersion: v1\nkind: ConfigMap",
			wantID:   ResourceID{APIVersion: "v1", Kind: "ConfigMap"},
			wantType: ConverterPassthrough,
		},
		{
			name:     "unknown CAPMOX kind passthrough",
			yaml:     "apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1\nkind: ProxmoxUnknown",
			wantID:   ResourceID{APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1", Kind: "ProxmoxUnknown"},
			wantType: ConverterPassthrough,
		},
		{
			name:     "empty document",
			yaml:     "",
			wantID:   ResourceID{},
			wantType: ConverterPassthrough,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotType := DetectResource([]byte(tt.yaml))
			if gotID != tt.wantID {
				t.Errorf("ID = %+v, want %+v", gotID, tt.wantID)
			}
			if gotType != tt.wantType {
				t.Errorf("type = %d, want %d", gotType, tt.wantType)
			}
		})
	}
}
