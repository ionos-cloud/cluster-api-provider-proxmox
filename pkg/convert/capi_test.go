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
)

func TestConvertCAPI_AllKinds(t *testing.T) {
	tests := []struct {
		apiVersion string
		kind       string
		wantAPI    string
	}{
		{"cluster.x-k8s.io/v1beta1", "Cluster", "cluster.x-k8s.io/v1beta2"},
		{"cluster.x-k8s.io/v1beta1", "MachineDeployment", "cluster.x-k8s.io/v1beta2"},
		{"controlplane.cluster.x-k8s.io/v1beta1", "KubeadmControlPlane", "controlplane.cluster.x-k8s.io/v1beta2"},
		{"bootstrap.cluster.x-k8s.io/v1beta1", "KubeadmConfigTemplate", "bootstrap.cluster.x-k8s.io/v1beta2"},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			input := `apiVersion: ` + tt.apiVersion + `
kind: ` + tt.kind + `
metadata:
  name: test
spec: {}
`
			id := ResourceID{APIVersion: tt.apiVersion, Kind: tt.kind}
			out, err := ConvertCAPI([]byte(input), id, testfile, 2, noopWarn, nil)
			if err != nil {
				t.Fatalf("ConvertCAPI %s: %v", tt.kind, err)
			}
			if !strings.Contains(string(out), tt.wantAPI) {
				t.Errorf("output should contain %s for %s", tt.wantAPI, tt.kind)
			}
		})
	}
}

func TestConvertCAPI_UnknownKind(t *testing.T) {
	input := `apiVersion: cluster.x-k8s.io/v1beta1
kind: UnknownKind
`
	id := ResourceID{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "UnknownKind"}
	_, err := ConvertCAPI([]byte(input), id, testfile, 2, noopWarn, nil)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "unknown CAPI resource") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConvertCAPI_InvalidYAML(t *testing.T) {
	input := `not: valid: yaml: [}`
	id := ResourceID{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Cluster"}
	_, err := ConvertCAPI([]byte(input), id, testfile, 2, noopWarn, nil)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestConvertCAPI_DoubleUnmarshalFailure(t *testing.T) {
	input := []byte("{{{{")
	id := ResourceID{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Cluster"}
	_, err := ConvertCAPI(input, id, testfile, 2, noopWarn, nil)
	if err == nil {
		t.Fatal("expected error for completely malformed YAML")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestConvertCAPI_MinimalValidInput(t *testing.T) {
	input := `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test
  # a comment
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
        - 10.0.0.0/16
`
	id := ResourceID{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Cluster"}
	out, err := ConvertCAPI([]byte(input), id, testfile, 2, noopWarn, nil)
	if err != nil {
		t.Fatalf("ConvertCAPI: %v", err)
	}
	result := string(out)
	if !strings.Contains(result, "v1beta2") {
		t.Error("output should contain v1beta2")
	}
	if !strings.Contains(result, "a comment") {
		t.Error("comment should be preserved")
	}
}

func TestConvertCAPI_StrictUnmarshalFallback(t *testing.T) {
	input := `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test
spec:
  unknownExtraField: value
`
	id := ResourceID{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Cluster"}
	out, err := ConvertCAPI([]byte(input), id, testfile, 2, noopWarn, nil)
	if err != nil {
		t.Fatalf("ConvertCAPI with extra field: %v", err)
	}
	if !strings.Contains(string(out), "v1beta2") {
		t.Error("output should contain v1beta2 after fallback unmarshal")
	}
}
