/*
Copyright 2024 IONOS Cloud.

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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

// ProxmoxClusterTemplateSpec defines the desired state of ProxmoxClusterTemplate.
type ProxmoxClusterTemplateSpec struct {
	// template is the Proxmox Cluster template
	// +required
	Template ProxmoxClusterTemplateResource `json:"template,omitzero"`
}

// ProxmoxClusterTemplateResource defines the spec and metadata for ProxmoxClusterTemplate supported by capi.
type ProxmoxClusterTemplateResource struct {
	// metadata is the standard object metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"` //nolint:kubeapilinter

	// spec is the Proxmox Cluster spec
	// +required
	Spec ProxmoxClusterClassTemplateSpec `json:"spec,omitzero"`
}

// ProxmoxClusterClassSpec defines the Cluster and its machine resources for Cluster Classes.
type ProxmoxClusterClassTemplateSpec struct {
	// ProxmoxClusterSpec is used as a template for Clusters.
	ProxmoxClusterSpec `json:",inline"`

	// ProxomxClusterCloneSpec holds all data required to clone proxmox machines.
	ProxmoxClusterCloneSpec `json:",inline"`
}

// ProxmoxClusterClassSpec defines deployment templates for ClusterClass.
type ProxmoxClusterClassSpec struct {
	// machineType is the name of the template for ClusterClass.
	// +required
	// +kubebuilder:validation:MinLength=1
	MachineType string `json:"machineType,omitempty"`

	// proxmoxMachineSpec is the to be patched yaml object.
	ProxmoxMachineSpec `json:",inline"`
}

// ProxmoxClusterCloneSpec is the configuration pertaining to all items configurable
// in the configuration and cloning of a proxmox VM.
type ProxmoxClusterCloneSpec struct {
	// machineSpec is the map of machine specs.
	// +listType=map
	// +listMapKey=machineType
	// +required
	ProxmoxClusterClassSpec []ProxmoxClusterClassSpec `json:"machineSpec,omitempty,omitzero"`

	// sshAuthorizedKeys contains the authorized keys deployed to the PROXMOX VMs.
	// +listType=set
	// +optional
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitzero"`

	// virtualIPNetworkInterface is the interface the k8s control plane binds to.
	// +optional
	VirtualIPNetworkInterface *string `json:"virtualIPNetworkInterface,omitempty,omitzero"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=proxmoxclustertemplates,scope=Namespaced,categories=cluster-api,shortName=pct
// +kubebuilder:storageversion

// ProxmoxClusterTemplate is the Schema for the proxmoxclustertemplates API.
type ProxmoxClusterTemplate struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the Proxmox Cluster Template spec
	// +required
	Spec ProxmoxClusterTemplateSpec `json:"spec,omitzero"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// ProxmoxClusterTemplateList contains a list of ProxmoxClusterTemplate.
type ProxmoxClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxClusterTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ProxmoxClusterTemplate{}, &ProxmoxClusterTemplateList{})
}
