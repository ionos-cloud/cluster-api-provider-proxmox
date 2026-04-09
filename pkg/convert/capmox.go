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

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	k8syaml "sigs.k8s.io/yaml"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

const capmoxV1Alpha2 = "infrastructure.cluster.x-k8s.io/v1alpha2"

// ConvertCAPMOX converts a CAPMOX v1alpha1 resource to v1alpha2 using typed conversion.
// Warnings are emitted immediately via warn.
func ConvertCAPMOX(yamlDoc []byte, id ResourceID, filename string, indent int, warn WarnFunc, entries []SentinelEntry) ([]byte, error) { //nolint:revive // name is clear
	src, dst, err := capmoxObjects(id.Kind)
	if err != nil {
		return nil, err
	}

	if err := unmarshalLoose(yamlDoc, src, id.Kind); err != nil {
		return nil, err
	}

	if err := convertObject(src, dst, id.Kind); err != nil {
		return nil, err
	}

	setTypeMeta(dst, capmoxV1Alpha2, id.Kind)

	return finalizeYAML(yamlDoc, dst, id.Kind, filename, indent, warn, entries)
}

func capmoxObjects(kind string) (runtime.Object, runtime.Object, error) {
	switch kind {
	case KindProxmoxCluster:
		return &v1alpha1.ProxmoxCluster{}, &v1alpha2.ProxmoxCluster{}, nil
	case KindProxmoxMachine:
		return &v1alpha1.ProxmoxMachine{}, &v1alpha2.ProxmoxMachine{}, nil
	case KindProxmoxMachineTemplate:
		return &v1alpha1.ProxmoxMachineTemplate{}, &v1alpha2.ProxmoxMachineTemplate{}, nil
	case KindProxmoxClusterTemplate:
		return &v1alpha1.ProxmoxClusterTemplate{}, &v1alpha2.ProxmoxClusterTemplate{}, nil
	default:
		return nil, nil, fmt.Errorf("unknown CAPMOX kind: %s", kind)
	}
}

func setTypeMeta(obj runtime.Object, apiVersion, kind string) {
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)
	obj.GetObjectKind().SetGroupVersionKind(gvk)
}

// unmarshalLoose tries strict unmarshal first, falls back to non-strict.
func unmarshalLoose(data []byte, dst runtime.Object, kind string) error {
	if err := k8syaml.UnmarshalStrict(data, dst); err != nil {
		if err2 := k8syaml.Unmarshal(data, dst); err2 != nil {
			return fmt.Errorf("unmarshal %s: %w", kind, err2)
		}
	}
	return nil
}

// convertObject runs ConvertTo from src (spoke) to dst (hub).
func convertObject(src, dst runtime.Object, kind string) error {
	convertible, ok := src.(conversion.Convertible)
	if !ok {
		return fmt.Errorf("%s does not implement Convertible", kind)
	}
	hub, ok := dst.(conversion.Hub)
	if !ok {
		return fmt.Errorf("%s does not implement Hub", kind)
	}
	if err := convertible.ConvertTo(hub); err != nil {
		return fmt.Errorf("ConvertTo %s: %w", kind, err)
	}
	return nil
}

// finalizeYAML marshals the converted object to YAML, grafts comments, prunes
// defaults, restores sentinels, and applies indentation. RestoreNode runs on
// the node tree (correct for block scalars); Restore runs on the text output
// afterwards to handle array sentinels that expanded to sequence nodes.
func finalizeYAML(srcYAML []byte, obj runtime.Object, kind, filename string, indent int, warn WarnFunc, entries []SentinelEntry) ([]byte, error) {
	outJSON, err := k8syaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", kind, err)
	}

	var srcNode, dstNode yaml.Node
	if err := yaml.Unmarshal(srcYAML, &srcNode); err != nil {
		return []byte(Restore(string(outJSON), entries)), nil //nolint:nilerr // graceful fallback: return without comments
	}
	if err := yaml.Unmarshal(outJSON, &dstNode); err != nil {
		return []byte(Restore(string(outJSON), entries)), nil //nolint:nilerr // graceful fallback: return without comments
	}

	GraftComments(&srcNode, &dstNode, filename, warn)
	PruneDefaults(&dstNode, kind)
	StripStatus(&dstNode, filename, warn)
	RestoreNode(&dstNode, entries)

	out, err := marshalWithIndent(&dstNode, indent)
	if err != nil {
		return []byte(Restore(string(outJSON), entries)), nil //nolint:nilerr // graceful fallback: return without indent
	}
	return []byte(Restore(string(out), entries)), nil
}
