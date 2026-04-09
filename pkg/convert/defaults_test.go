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
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPruneDefaults_NullFields(t *testing.T) {
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: ProxmoxCluster
spec:
  controlPlaneEndpoint:
    host: 10.0.0.1
    port: 6443
  zoneConfigs: null
  externalManagedControlPlane: null
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "ProxmoxCluster")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "zoneConfigs") {
		t.Error("zoneConfigs should be pruned")
	}
	if strings.Contains(result, "externalManagedControlPlane") {
		t.Error("externalManagedControlPlane should be pruned")
	}
	if !strings.Contains(result, "controlPlaneEndpoint") {
		t.Error("controlPlaneEndpoint should be preserved")
	}
}

func TestPruneDefaults_KeepsNonDefault(t *testing.T) {
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: ProxmoxCluster
spec:
  externalManagedControlPlane: true
  allowedNodes:
    - node1
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "ProxmoxCluster")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if !strings.Contains(result, "externalManagedControlPlane: true") {
		t.Error("non-default externalManagedControlPlane should be preserved")
	}
	if !strings.Contains(result, "allowedNodes") {
		t.Error("allowedNodes should be preserved")
	}
}

func TestPruneDefaults_UnknownKind(t *testing.T) {
	input := `kind: Unknown
spec:
  field: null
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	// Should still prune nulls generically.
	PruneDefaults(&node, "Unknown")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "field") {
		t.Error("null field should be pruned even for unknown kind")
	}
}

func TestPruneDefaults_DeepNulls(t *testing.T) {
	input := `spec:
  network:
    default:
      bridge: vmbr0
      mtu: null
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "ProxmoxMachine")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "mtu") {
		t.Error("deep null mtu should be pruned")
	}
	if !strings.Contains(result, "bridge: vmbr0") {
		t.Error("non-null bridge should be preserved")
	}
}

func TestPruneDefaults_EmptyMappingPruned(t *testing.T) {
	input := `apiVersion: v1
spec:
  nested:
    inner: null
  other: value
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "ProxmoxMachine")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	// inner was null → pruned, then nested becomes empty mapping → pruned.
	if strings.Contains(result, "nested") {
		t.Error("empty nested mapping should be pruned")
	}
	if !strings.Contains(result, "other: value") {
		t.Error("non-empty field should be preserved")
	}
}

func TestPruneDefaults_SequenceWithNulls(t *testing.T) {
	input := `items:
  - name: a
    value: null
  - name: b
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "Unknown")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "value") {
		t.Error("null value inside sequence item should be pruned")
	}
	if !strings.Contains(result, "name: a") || !strings.Contains(result, "name: b") {
		t.Error("non-null fields in sequence should be preserved")
	}
}

func TestPruneDefaults_FalseDefaultProxmoxCluster(t *testing.T) {
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: ProxmoxCluster
spec:
  externalManagedControlPlane: false
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "ProxmoxCluster")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "externalManagedControlPlane") {
		t.Error("false externalManagedControlPlane should be pruned for ProxmoxCluster")
	}
}

func TestPruneDefaults_EmptyStringDefault(t *testing.T) {
	input := `spec:
  externalManagedControlPlane: ""
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "ProxmoxCluster")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "externalManagedControlPlane") {
		t.Error("empty string externalManagedControlPlane should be pruned for ProxmoxCluster")
	}
}

func TestPruneDefaults_NilNode(_ *testing.T) {
	// Should not panic.
	PruneDefaults(nil, "ProxmoxCluster")
}

func TestMatchesDefault_NonScalar(t *testing.T) {
	node := &yaml.Node{Kind: yaml.MappingNode}
	if matchesDefault(node, "null") {
		t.Error("non-scalar should not match any default")
	}
}

func TestPruneDefaults_ProxmoxClusterTemplate(t *testing.T) {
	input := `spec:
  zoneConfigs: null
  externalManagedControlPlane: false
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "ProxmoxClusterTemplate")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "zoneConfigs") {
		t.Error("zoneConfigs should be pruned for ProxmoxClusterTemplate")
	}
	if strings.Contains(result, "externalManagedControlPlane") {
		t.Error("false externalManagedControlPlane should be pruned for ProxmoxClusterTemplate")
	}
}

func TestPruneDefaults_SequenceInMapping(t *testing.T) {
	// Test pruneEmptyMappingsDeep with sequence children.
	input := `items:
  - spec:
      inner: null
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	PruneDefaults(&node, "Unknown")

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	// After pruning null + empty mapping, the sequence item becomes empty.
	result := string(out)
	_ = result
}
