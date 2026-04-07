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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	bootstrapv1beta2 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	kcpv1beta1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta1"
	kcpv1beta2 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// capiVersionMap maps v1beta1 apiVersion → v1beta2 apiVersion.
var capiVersionMap = map[string]string{
	"cluster.x-k8s.io/v1beta1":              "cluster.x-k8s.io/v1beta2",
	"controlplane.cluster.x-k8s.io/v1beta1": "controlplane.cluster.x-k8s.io/v1beta2",
	"bootstrap.cluster.x-k8s.io/v1beta1":    "bootstrap.cluster.x-k8s.io/v1beta2",
}

// ConvertCAPI converts a CAPI v1beta1 resource to v1beta2 using native ConvertTo methods.
// Warnings are emitted immediately via warn.
func ConvertCAPI(yamlDoc []byte, id ResourceID, filename string, indent int, warn WarnFunc) ([]byte, error) { //nolint:revive // name is clear
	src, dst, err := capiObjects(id.APIVersion, id.Kind)
	if err != nil {
		return nil, err
	}

	if err := unmarshalLoose(yamlDoc, src, id.Kind); err != nil {
		return nil, err
	}

	if err := convertObject(src, dst, id.Kind); err != nil {
		return nil, err
	}

	newAPIVersion := capiVersionMap[id.APIVersion]
	setTypeMeta(dst, newAPIVersion, id.Kind)

	return finalizeYAML(yamlDoc, dst, id.Kind, filename, indent, warn)
}

func capiObjects(apiVersion, kind string) (runtime.Object, runtime.Object, error) {
	key := apiVersion + "/" + kind
	switch key {
	case "cluster.x-k8s.io/v1beta1/Cluster":
		return &clusterv1.Cluster{}, &clusterv1beta2.Cluster{}, nil
	case "cluster.x-k8s.io/v1beta1/MachineDeployment":
		return &clusterv1.MachineDeployment{}, &clusterv1beta2.MachineDeployment{}, nil
	case "controlplane.cluster.x-k8s.io/v1beta1/KubeadmControlPlane":
		return &kcpv1beta1.KubeadmControlPlane{}, &kcpv1beta2.KubeadmControlPlane{}, nil
	case "bootstrap.cluster.x-k8s.io/v1beta1/KubeadmConfigTemplate":
		return &bootstrapv1beta1.KubeadmConfigTemplate{}, &bootstrapv1beta2.KubeadmConfigTemplate{}, nil
	default:
		return nil, nil, fmt.Errorf("unknown CAPI resource: %s %s", apiVersion, kind)
	}
}
