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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ProxmoxMachineTemplateSpec defines the desired state of ProxmoxMachineTemplate.
type ProxmoxMachineTemplateSpec struct {
	Template ProxmoxMachineTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=proxmoxmachinetemplates,scope=Namespaced,categories=cluster-api,shortName=pmt
// +kubebuilder:storageversion

// ProxmoxMachineTemplate is the Schema for the proxmoxmachinetemplates API.
type ProxmoxMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ProxmoxMachineTemplateSpec `json:"spec,omitempty"`
}

// ProxmoxMachineTemplateResource defines the spec and metadata for ProxmoxMachineTemplate supported by capi.
type ProxmoxMachineTemplateResource struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`
	Spec       ProxmoxMachineSpec   `json:"spec"`
}

//+kubebuilder:object:root=true

// ProxmoxMachineTemplateList contains a list of ProxmoxMachineTemplate.
type ProxmoxMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxMachineTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ProxmoxMachineTemplate{}, &ProxmoxMachineTemplateList{})
}
