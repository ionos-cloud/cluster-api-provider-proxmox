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
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// noopWarn is a dummy warning handler for tests that don't inspect warnings.
func noopWarn(Warning) { /* dummy */ }

const testfile = "test.yaml"

const configMapYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`

const proxmoxClusterV1Alpha1YAML = `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: test
spec:
  controlPlaneEndpoint:
    host: 10.0.0.1
    port: 6443
`

func TestConvert_V1Alpha1Template(t *testing.T) {
	input, err := os.ReadFile("testdata/v1alpha1-cluster-template.yaml")
	if err != nil {
		t.Fatalf("reading test fixture: %v", err)
	}

	var warnings []Warning
	out, err := Convert(input, Options{
		Filename: "cluster-template.yaml",
		Warn: func(w Warning) {
			warnings = append(warnings, w)
			t.Logf("WARNING: %s", w)
		},
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	result := string(out)

	// Verify apiVersion bumps.
	if strings.Contains(result, "infrastructure.cluster.x-k8s.io/v1alpha1") {
		t.Error("output still contains v1alpha1 CAPMOX apiVersion")
	}
	if !strings.Contains(result, "infrastructure.cluster.x-k8s.io/v1alpha2") {
		t.Error("output missing v1alpha2 CAPMOX apiVersion")
	}
	if strings.Contains(result, "cluster.x-k8s.io/v1beta1") {
		t.Error("output still contains v1beta1 CAPI apiVersion")
	}

	// Verify all ${VAR} expressions are preserved.
	inputVars := findEnvsubstVars(string(input))
	outputVars := findEnvsubstVars(result)
	for v := range inputVars {
		if !outputVars[v] {
			t.Errorf("envsubst var %s lost during conversion", v)
		}
	}

	// Verify no top-level null fields leaked (exclude block scalars like embedded manifests).
	for _, line := range strings.Split(result, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip lines inside block scalars (indented content or literal blocks).
		if strings.HasSuffix(trimmed, ": null") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			// Check this isn't inside a block scalar by seeing if it's a YAML key.
			// Block scalar content is typically indented further than the surrounding keys.
			indent := len(line) - len(strings.TrimLeft(line, " "))
			if indent <= 8 { // top-level YAML keys are indented ≤ 8 spaces
				t.Errorf("output contains null field: %s", trimmed)
			}
		}
	}
}

func TestConvert_V1Alpha1Template_GoldenFile(t *testing.T) {
	input, err := os.ReadFile("testdata/v1alpha1-cluster-template.yaml")
	if err != nil {
		t.Fatalf("reading input fixture: %v", err)
	}

	expected, err := os.ReadFile("testdata/v1alpha2-cluster-template.yaml")
	if err != nil {
		t.Fatalf("reading golden fixture: %v", err)
	}

	out, err := Convert(input, Options{
		Filename: "cluster-template.yaml",
		Warn:     noopWarn,
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if string(out) != string(expected) {
		t.Errorf("output does not match golden file testdata/v1alpha2-cluster-template.yaml\n"+
			"diff (-want +got):\n%s", unifiedDiff(string(expected), string(out)))
	}
}

func TestConvert_Passthrough(t *testing.T) {
	input := configMapYAML + `data:
  key: value
`

	var warnings []Warning
	out, err := Convert([]byte(input), Options{
		Filename: testfile,
		Warn: func(w Warning) {
			warnings = append(warnings, w)
		},
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if string(out) != input {
		t.Error("passthrough should return input unchanged")
	}

	if len(warnings) != 1 {
		t.Errorf("expected 1 passthrough warning, got %d", len(warnings))
	} else if warnings[0].Kind != "passthrough" {
		t.Errorf("expected passthrough warning, got %s", warnings[0].Kind)
	}
}

func TestConvert_MultiDoc(t *testing.T) {
	input := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
---
apiVersion: v1
kind: Secret
metadata:
  name: test
`

	passthroughCount := 0
	_, err := Convert([]byte(input), Options{
		Warn: func(w Warning) {
			if w.Kind == "passthrough" {
				passthroughCount++
			}
		},
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if passthroughCount != 2 {
		t.Errorf("expected 2 passthrough warnings, got %d", passthroughCount)
	}
}

func TestConvert_NilWarn(t *testing.T) {
	// nil Warn should not panic.
	out, err := Convert([]byte(configMapYAML), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(string(out), "ConfigMap") {
		t.Error("output should contain ConfigMap")
	}
}

func TestConvert_ExplicitIndent(t *testing.T) {
	// 2-space indented input, request 4-space output.
	out, err := Convert([]byte(configMapYAML), Options{Indent: 4})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// Passthrough returns unchanged, so test with a real conversion.
	_ = out
}

func TestConvert_ExplicitIndent_CAPMOX(t *testing.T) {
	out, err := Convert([]byte(proxmoxClusterV1Alpha1YAML), Options{Indent: 4})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// Check that 4-space indent is used.
	result := string(out)
	if !strings.Contains(result, "    name: test") {
		t.Errorf("expected 4-space indent in output:\n%s", result)
	}
}

func TestConvert_EmptyInput(t *testing.T) {
	out, err := Convert([]byte(""), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output, got %q", out)
	}
}

func TestConvert_EmptyDocSeparator(t *testing.T) {
	input := "---\n---\n"
	out, err := Convert([]byte(input), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if string(out) != input {
		t.Errorf("expected %q, got %q", input, string(out))
	}
}

func TestConvert_DefaultFilename(t *testing.T) {
	var warnings []Warning
	_, err := Convert([]byte(configMapYAML), Options{
		Warn: func(w Warning) {
			warnings = append(warnings, w)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].File != "<stdin>" {
		t.Errorf("expected default filename <stdin>, got %q", warnings[0].File)
	}
}

func TestConvert_SingleCAPMOXResource(t *testing.T) {
	kinds := []string{
		"ProxmoxCluster",
		"ProxmoxMachine",
		"ProxmoxMachineTemplate",
		"ProxmoxClusterTemplate",
	}
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
			out, err := Convert([]byte(input), Options{
				Warn: noopWarn,
			})
			if err != nil {
				t.Fatalf("Convert %s: %v", kind, err)
			}
			result := string(out)
			if !strings.Contains(result, "v1alpha2") {
				t.Errorf("expected v1alpha2 in output for %s", kind)
			}
			if strings.Contains(result, "v1alpha1") {
				t.Errorf("v1alpha1 should not be in output for %s", kind)
			}
		})
	}
}

func TestConvert_SingleCAPIResource(t *testing.T) {
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
			out, err := Convert([]byte(input), Options{
				Warn: noopWarn,
			})
			if err != nil {
				t.Fatalf("Convert %s: %v", tt.kind, err)
			}
			result := string(out)
			if !strings.Contains(result, tt.wantAPI) {
				t.Errorf("expected %s in output for %s, got:\n%s", tt.wantAPI, tt.kind, result)
			}
		})
	}
}

func TestConvert_PassthroughNoAPIVersion(t *testing.T) {
	// A document with no apiVersion/kind should pass through without warnings.
	input := `# just a comment
key: value
`
	var warnings []Warning
	out, err := Convert([]byte(input), Options{
		Warn: func(w Warning) {
			warnings = append(warnings, w)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != input {
		t.Errorf("expected passthrough, got %q", string(out))
	}
	// No apiVersion/kind means no passthrough warning.
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for bare YAML, got %d", len(warnings))
	}
}

func TestDetectIndent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect int
	}{
		{"one space", "apiVersion: v1\nkind: Test\nspec:\n name: foo\n", 1},
		{"two spaces", "apiVersion: v1\nkind: Test\nspec:\n  name: foo\n", 2},
		{"three spaces", "apiVersion: v1\nkind: Test\nspec:\n   name: foo\n", 3},
		{"four spaces", "apiVersion: v1\nkind: Test\nspec:\n    name: foo\n", 4},
		{"eight spaces", "apiVersion: v1\nkind: Test\nspec:\n        name: foo\n", 8},
		{"nine spaces defaults to 2", "apiVersion: v1\nkind: Test\nspec:\n         name: foo\n", 2},
		{"default on flat", "apiVersion: v1\nkind: Test\n", 2},
		{"tabs ignored", "apiVersion: v1\n\tname: foo\n  real: indent\n", 2},
		{"empty input", "", 2},
		{"only empty lines", "\n\n\n", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectIndent([]byte(tt.input))
			if got != tt.expect {
				t.Errorf("detectIndent = %d, want %d", got, tt.expect)
			}
		})
	}
}

func TestMarshalWithIndent(t *testing.T) {
	for _, indent := range []int{2, 4} {
		t.Run(strings.Repeat(" ", indent)+"indent", func(t *testing.T) {
			out, err := Convert([]byte(proxmoxClusterV1Alpha1YAML), Options{
				Indent: indent,
				Warn:   noopWarn,
			})
			if err != nil {
				t.Fatal(err)
			}
			expected := strings.Repeat(" ", indent) + "name: test"
			if !strings.Contains(string(out), expected) {
				t.Errorf("expected %d-space indent in output:\n%s", indent, out)
			}
		})
	}
}

func TestMarshalWithIndent_Direct(t *testing.T) {
	var node yaml.Node
	input := `apiVersion: v1
kind: Test
spec:
  name: foo
`
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}

	out, err := marshalWithIndent(&node, 2)
	if err != nil {
		t.Fatalf("marshalWithIndent: %v", err)
	}
	if !strings.Contains(string(out), "  name: foo") {
		t.Errorf("expected 2-space indent, got:\n%s", out)
	}

	out4, err := marshalWithIndent(&node, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out4), "    name: foo") {
		t.Errorf("expected 4-space indent, got:\n%s", out4)
	}
}

func TestSplitYAMLDocuments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantDocs int
	}{
		{"single doc no separator", "apiVersion: v1\nkind: ConfigMap\n", 1},
		{"single doc with separator", "---\napiVersion: v1\nkind: ConfigMap\n", 2}, // separator + doc
		{"two docs", "---\napiVersion: v1\nkind: A\n---\napiVersion: v1\nkind: B\n", 4},
		{"empty", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := splitYAMLDocuments([]byte(tt.input))
			if len(docs) != tt.wantDocs {
				t.Errorf("splitYAMLDocuments got %d docs, want %d", len(docs), tt.wantDocs)
			}
		})
	}
}

var envsubstVarRe = regexp.MustCompile(`\$\{[^}]+\}`)

func findEnvsubstVars(text string) map[string]bool {
	matches := envsubstVarRe.FindAllString(text, -1)
	result := make(map[string]bool)
	for _, m := range matches {
		result[m] = true
	}
	return result
}

// unifiedDiff returns a simple line-by-line diff for test failure messages.
func unifiedDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")

	var b strings.Builder
	maxLines := len(wantLines)
	if len(gotLines) > maxLines {
		maxLines = len(gotLines)
	}
	for i := 0; i < maxLines; i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			b.WriteString("- " + w + "\n")
			b.WriteString("+ " + g + "\n")
		}
	}
	return b.String()
}
