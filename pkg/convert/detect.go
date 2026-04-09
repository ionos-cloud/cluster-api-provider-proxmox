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

import (
	"gopkg.in/yaml.v3"
)

// ResourceID identifies a Kubernetes resource by apiVersion and kind.
type ResourceID struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

// ConverterType indicates which conversion path to use.
type ConverterType int

// Converter types for routing documents to the correct conversion pipeline.
const (
	ConverterPassthrough ConverterType = iota
	ConverterCAPMOX
	ConverterCAPI
)

// CAPMOX resource kind constants.
const (
	KindProxmoxCluster         = "ProxmoxCluster"
	KindProxmoxMachine         = "ProxmoxMachine"
	KindProxmoxMachineTemplate = "ProxmoxMachineTemplate"
	KindProxmoxClusterTemplate = "ProxmoxClusterTemplate"
)

// DetectResource partially unmarshals a YAML document to determine its resource type
// and returns the appropriate converter type.
func DetectResource(yamlDoc []byte) (ResourceID, ConverterType) {
	var id ResourceID
	_ = yaml.Unmarshal(yamlDoc, &id)

	switch id.APIVersion {
	case "infrastructure.cluster.x-k8s.io/v1alpha1":
		switch id.Kind {
		case KindProxmoxCluster, KindProxmoxMachine, KindProxmoxMachineTemplate, KindProxmoxClusterTemplate:
			return id, ConverterCAPMOX
		}

	case "cluster.x-k8s.io/v1beta1":
		switch id.Kind {
		case "Cluster", "MachineDeployment":
			return id, ConverterCAPI
		}

	case "controlplane.cluster.x-k8s.io/v1beta1":
		if id.Kind == "KubeadmControlPlane" {
			return id, ConverterCAPI
		}

	case "bootstrap.cluster.x-k8s.io/v1beta1":
		if id.Kind == "KubeadmConfigTemplate" {
			return id, ConverterCAPI
		}
	}

	return id, ConverterPassthrough
}
