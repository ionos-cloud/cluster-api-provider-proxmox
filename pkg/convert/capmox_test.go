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

	"github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

func TestConvertCAPMOX_AllKinds(t *testing.T) {
	kinds := []string{"ProxmoxCluster", "ProxmoxMachine", "ProxmoxMachineTemplate", "ProxmoxClusterTemplate"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ` + kind + `
metadata:
  name: test
spec:
  controlPlaneEndpoint:
    host: 10.0.0.1
    port: 6443
`
			id := ResourceID{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
				Kind:       kind,
			}
			out, err := ConvertCAPMOX([]byte(input), id, testfile, 2, noopWarn)
			if err != nil {
				t.Fatalf("ConvertCAPMOX %s: %v", kind, err)
			}
			if !strings.Contains(string(out), "v1alpha2") {
				t.Errorf("output should contain v1alpha2 for %s", kind)
			}
		})
	}
}

func TestConvertCAPMOX_UnknownKind(t *testing.T) {
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxUnknown
`
	id := ResourceID{
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
		Kind:       "ProxmoxUnknown",
	}
	_, err := ConvertCAPMOX([]byte(input), id, testfile, 2, noopWarn)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "unknown CAPMOX kind") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConvertCAPMOX_InvalidYAML(t *testing.T) {
	input := `not: valid: yaml: [}`
	id := ResourceID{
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
		Kind:       "ProxmoxCluster",
	}
	_, err := ConvertCAPMOX([]byte(input), id, testfile, 2, noopWarn)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestConvertCAPMOX_DoubleUnmarshalFailure(t *testing.T) {
	// Completely malformed YAML that fails both strict and non-strict.
	input := []byte("{{{{")
	id := ResourceID{
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
		Kind:       "ProxmoxCluster",
	}
	_, err := ConvertCAPMOX(input, id, testfile, 2, noopWarn)
	if err == nil {
		t.Fatal("expected error for completely malformed YAML")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestUnmarshalLoose_StrictOK(t *testing.T) {
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: test
`
	var obj v1alpha1.ProxmoxCluster
	err := unmarshalLoose([]byte(input), &obj, "ProxmoxCluster")
	if err != nil {
		t.Fatalf("unmarshalLoose: %v", err)
	}
	if obj.Name != "test" {
		t.Errorf("name = %q, want test", obj.Name)
	}
}

func TestUnmarshalLoose_StrictFailsNonStrictOK(t *testing.T) {
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: test
unknownField: value
`
	var obj v1alpha1.ProxmoxCluster
	err := unmarshalLoose([]byte(input), &obj, "ProxmoxCluster")
	if err != nil {
		t.Fatalf("unmarshalLoose should fall back to non-strict: %v", err)
	}
}

func TestUnmarshalLoose_BothFail(t *testing.T) {
	err := unmarshalLoose([]byte("{{{{"), &v1alpha1.ProxmoxCluster{}, "ProxmoxCluster")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConvertObject_Success(t *testing.T) {
	src := &v1alpha1.ProxmoxCluster{}
	dst := &v1alpha2.ProxmoxCluster{}
	err := convertObject(src, dst, "ProxmoxCluster")
	if err != nil {
		t.Fatalf("convertObject: %v", err)
	}
}

func TestFinalizeYAML_HappyPath(t *testing.T) {
	src := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: ProxmoxCluster
metadata:
  name: test
  # a comment
`
	obj := &v1alpha2.ProxmoxCluster{}
	obj.Name = "test"
	obj.APIVersion = "infrastructure.cluster.x-k8s.io/v1alpha2"
	obj.Kind = "ProxmoxCluster"

	out, err := finalizeYAML([]byte(src), obj, "ProxmoxCluster", testfile, 2, noopWarn)
	if err != nil {
		t.Fatalf("finalizeYAML: %v", err)
	}
	if !strings.Contains(string(out), "name: test") {
		t.Error("output should contain name")
	}
}

func TestConvertCAPMOX_MinimalValidInput(t *testing.T) {
	// Minimal valid input that exercises the full happy path.
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: test
spec:
  controlPlaneEndpoint:
    host: 10.0.0.1
    port: 6443
`
	id := ResourceID{
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
		Kind:       "ProxmoxCluster",
	}
	out, err := ConvertCAPMOX([]byte(input), id, testfile, 2, noopWarn)
	if err != nil {
		t.Fatalf("ConvertCAPMOX: %v", err)
	}
	result := string(out)
	if !strings.Contains(result, "v1alpha2") {
		t.Error("output should contain v1alpha2")
	}
}

func TestConvertCAPMOX_StrictUnmarshalFallback(t *testing.T) {
	// Input with an extra unknown field triggers strict failure, non-strict succeeds.
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: test
spec:
  controlPlaneEndpoint:
    host: 10.0.0.1
    port: 6443
  unknownExtraField: value
`
	id := ResourceID{
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
		Kind:       "ProxmoxCluster",
	}
	out, err := ConvertCAPMOX([]byte(input), id, testfile, 2, noopWarn)
	if err != nil {
		t.Fatalf("ConvertCAPMOX with extra field: %v", err)
	}
	if !strings.Contains(string(out), "v1alpha2") {
		t.Error("output should contain v1alpha2 after fallback unmarshal")
	}
}
