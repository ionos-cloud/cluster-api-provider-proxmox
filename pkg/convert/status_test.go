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

func TestStripStatus_ZeroValueStatus(t *testing.T) {
	input := `apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  name: test
spec:
  replicas: 1
status:
  deprecated:
    v1beta1:
      availableReplicas: 0
      readyReplicas: 0
      unavailableReplicas: 0
      updatedReplicas: 0
  replicas: 0
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	var warnings []Warning
	warn := func(w Warning) { warnings = append(warnings, w) }

	StripStatus(&node, testfile, warn)

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "status") {
		t.Errorf("zero-value status block should be stripped, got:\n%s", result)
	}
	if !strings.Contains(result, "replicas: 1") {
		t.Error("spec.replicas should be preserved")
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(warnings))
	}
}

func TestStripStatus_NonZeroStatusKept(t *testing.T) {
	input := `apiVersion: v1
kind: Cluster
spec:
  name: test
status:
  phase: Running
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	var warnings []Warning
	warn := func(w Warning) { warnings = append(warnings, w) }

	StripStatus(&node, testfile, warn)

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if !strings.Contains(result, "status") {
		t.Error("non-zero status block should be kept")
	}
	if !strings.Contains(result, "phase: Running") {
		t.Error("status.phase should be preserved")
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Kind != "status" {
		t.Errorf("expected warning kind 'status', got %q", warnings[0].Kind)
	}
}

func TestStripStatus_NoStatusBlock(t *testing.T) {
	input := `apiVersion: v1
kind: Cluster
spec:
  name: test
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	var warnings []Warning
	warn := func(w Warning) { warnings = append(warnings, w) }

	StripStatus(&node, testfile, warn)

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if !strings.Contains(result, "name: test") {
		t.Error("spec should be preserved")
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(warnings))
	}
}

func TestStripStatus_NilNode(_ *testing.T) {
	// Should not panic on nil input.
	StripStatus(nil, testfile, func(Warning) { /* noop: verifying no panic */ })
}

func TestStripStatus_EmptyStatus(t *testing.T) {
	input := `apiVersion: v1
kind: Test
spec:
  name: test
status: {}
`

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	var warnings []Warning
	warn := func(w Warning) { warnings = append(warnings, w) }

	StripStatus(&node, testfile, warn)

	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if strings.Contains(result, "status") {
		t.Errorf("empty status block should be stripped, got:\n%s", result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(warnings))
	}
}
