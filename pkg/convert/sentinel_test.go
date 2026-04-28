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

func TestScanAndReplace_Basic(t *testing.T) {
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxMachine
spec:
  numSockets: ${NUM_SOCKETS:=2}
  numCores: ${NUM_CORES}
  memoryMiB: ${MEMORY_MIB:=4096}
  providerID: ${PROVIDER_ID}
`

	replaced, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}

	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Check type inference.
	entryMap := make(map[string]SentinelEntry)
	for _, e := range entries {
		entryMap[e.Original] = e
	}

	if e := entryMap["${NUM_SOCKETS:=2}"]; e.Type != typeInt {
		t.Errorf("NUM_SOCKETS type = %q, want %s", e.Type, typeInt)
	}
	if e := entryMap["${NUM_CORES}"]; e.Type != typeInt {
		t.Errorf("NUM_CORES type = %q, want %s (known key)", e.Type, typeInt)
	}
	if e := entryMap["${MEMORY_MIB:=4096}"]; e.Type != typeInt {
		t.Errorf("MEMORY_MIB type = %q, want %s", e.Type, typeInt)
	}
	if e := entryMap["${PROVIDER_ID}"]; e.Type != typeString {
		t.Errorf("PROVIDER_ID type = %q, want %s", e.Type, typeString)
	}

	// Verify replacement happened.
	for _, e := range entries {
		if strings.Contains(replaced, e.Original) {
			t.Errorf("replaced text still contains %q", e.Original)
		}
		if !strings.Contains(replaced, e.Sentinel) {
			t.Errorf("replaced text missing sentinel %q", e.Sentinel)
		}
	}
}

func TestScanAndReplace_BoolAndArray(t *testing.T) {
	input := `spec:
  enabled: ${SOME_BOOL:=true}
  nodes: ${ALLOWED_NODES:=[]}`

	_, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}

	entryMap := make(map[string]SentinelEntry)
	for _, e := range entries {
		entryMap[e.Original] = e
	}

	if e := entryMap["${SOME_BOOL:=true}"]; e.Type != typeBool {
		t.Errorf("SOME_BOOL type = %q, want %s", e.Type, typeBool)
	}
	if e := entryMap["${ALLOWED_NODES:=[]}"]; e.Type != typeArray {
		t.Errorf("ALLOWED_NODES type = %q, want %s", e.Type, typeArray)
	}
}

func TestScanAndReplace_Dedup(t *testing.T) {
	input := `name: ${CLUSTER_NAME}
namespace: ${CLUSTER_NAME}`

	_, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (dedup), got %d", len(entries))
	}
}

func TestRestore_RoundTrip(t *testing.T) {
	input := `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
spec:
  controlPlaneEndpoint:
    host: ${CONTROL_PLANE_ENDPOINT_IP}
    port: ${CONTROL_PLANE_ENDPOINT_PORT:=6443}
  allowedNodes:
    - ${NODE_1}
`

	replaced, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}

	restored := Restore(replaced, entries)
	if restored != input {
		t.Errorf("round-trip failed.\nGot:\n%s\nWant:\n%s", restored, input)
	}
}

func TestRestore_QuotedStringSentinel(t *testing.T) {
	entries := []SentinelEntry{
		{Original: "${CLUSTER_NAME}", Sentinel: "__SENTINEL_abcd1234__", Type: "string"},
	}

	// Restore replaces only the sentinel; surrounding " (whether YAML quoting or
	// literal content) are preserved in the output text.
	yamlText := `name: "__SENTINEL_abcd1234__"`
	restored := Restore(yamlText, entries)
	want := `name: "${CLUSTER_NAME}"`
	if restored != want {
		t.Errorf("Restore quoted string:\ngot:  %s\nwant: %s", restored, want)
	}
}

func TestRestore_QuotedIntSentinel(t *testing.T) {
	entries := []SentinelEntry{
		{Original: "${NUM_SOCKETS:=2}", Sentinel: "99900001", Type: "int"},
	}

	// Simulate YAML marshal quoting an integer.
	yamlText := `numSockets: "99900001"`
	restored := Restore(yamlText, entries)
	want := `numSockets: ${NUM_SOCKETS:=2}`
	if restored != want {
		t.Errorf("Restore quoted int:\ngot:  %s\nwant: %s", restored, want)
	}
}

func TestScanAndReplace_NoVars(t *testing.T) {
	input := `apiVersion: v1
kind: ConfigMap`

	replaced, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
	if replaced != input {
		t.Error("text should be unchanged when no vars")
	}
}

func TestScanAndReplace_PartialString(t *testing.T) {
	input := `name: ${CLUSTER_NAME}-worker`

	replaced, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Should contain sentinel followed by -worker.
	if !strings.Contains(replaced, entries[0].Sentinel+"-worker") {
		t.Errorf("partial string replacement broken: %s", replaced)
	}

	// Restore should bring back original.
	restored := Restore(replaced, entries)
	if restored != input {
		t.Errorf("partial string round-trip:\ngot:  %s\nwant: %s", restored, input)
	}
}

func TestExtractDefault(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{"${VAR}", ""},
		{"${VAR:=hello}", "hello"},
		{"${NUM:=42}", "42"},
		{"${BOOL:=true}", "true"},
		{"${ARR:=[]}", "[]"},
		{"${COMPLEX:=some:value}", "some:value"},
	}

	for _, tt := range tests {
		got := extractDefault(tt.expr)
		if got != tt.want {
			t.Errorf("extractDefault(%q) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}

func TestExtractYAMLKey(t *testing.T) {
	tests := []struct {
		name string
		expr string
		yaml string
		want string
	}{
		{"simple key", "${VAR}", "name: ${VAR}", "name"},
		{"indented key", "${VAR}", "  replicas: ${VAR}", "replicas"},
		{"expr not found", "${MISSING}", "name: ${VAR}", ""},
		{"no colon", "${VAR}", "- ${VAR}", ""},
		{"first line", "${VAR}", "name: ${VAR}\nother: foo", "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractYAMLKey(tt.expr, tt.yaml)
			if got != tt.want {
				t.Errorf("extractYAMLKey(%q, ...) = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

func TestIsWholeValue(t *testing.T) {
	tests := []struct {
		name string
		expr string
		yaml string
		want bool
	}{
		{"whole value", "${VAR}", "name: ${VAR}", true},
		{"with brackets", "${VAR}", "items: [${VAR}]", false},
		{"partial string", "${VAR}", "name: ${VAR}-suffix", false},
		{"not found", "${MISSING}", "name: ${VAR}", false},
		{"no colon", "${VAR}", "- ${VAR}", false},
		{"end of file no newline", "${VAR}", "name: ${VAR}", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWholeValue(tt.expr, tt.yaml)
			if got != tt.want {
				t.Errorf("isWholeValue(%q, ...) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestExtractArraySentinelInner(t *testing.T) {
	tests := []struct {
		name     string
		sentinel string
		want     string
	}{
		{"valid", `["__SENTARR_abc__"]`, "__SENTARR_abc__"},
		{"not an array sentinel", "__SENTINEL_abc__", ""},
		{"partial prefix only", `["partial`, "partial"},
		{"no prefix at all", `something_else`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractArraySentinelInner(tt.sentinel)
			if got != tt.want {
				t.Errorf("extractArraySentinelInner(%q) = %q, want %q", tt.sentinel, got, tt.want)
			}
		})
	}
}

func TestInferType(t *testing.T) {
	tests := []struct {
		name string
		expr string
		yaml string
		want string
	}{
		{"int default", "${NUM:=42}", "val: ${NUM:=42}", typeInt},
		{"bool default true", "${B:=true}", "val: ${B:=true}", typeBool},
		{"bool default false", "${B:=false}", "val: ${B:=false}", typeBool},
		{"array default", "${A:=[]}", "val: ${A:=[]}", typeArray},
		{"string default", "${S:=hello}", "val: ${S:=hello}", typeString},
		{"known int key", "${VAR}", "replicas: ${VAR}", typeInt},
		{"known array key whole value", "${VAR}", "dnsServers: ${VAR}", typeArray},
		{"known array key not whole value", "${VAR}", "dnsServers: [${VAR}]", typeString},
		{"unknown key fallback", "${VAR}", "foo: ${VAR}", typeString},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferType(tt.expr, tt.yaml)
			if got != tt.want {
				t.Errorf("inferType(%q, ...) = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

func TestRestore_BoolSentinel(t *testing.T) {
	entries := []SentinelEntry{
		{Original: "${ENABLED}", Sentinel: "true", Type: "bool"},
	}
	// Quoted: surrounding " are preserved; only the sentinel itself is replaced.
	yamlText := `enabled: "true"`
	restored := Restore(yamlText, entries)
	want := `enabled: "${ENABLED}"`
	if restored != want {
		t.Errorf("Restore bool quoted:\ngot:  %s\nwant: %s", restored, want)
	}

	// Unquoted bool sentinel.
	yamlText2 := `enabled: true`
	restored2 := Restore(yamlText2, entries)
	want2 := `enabled: ${ENABLED}`
	if restored2 != want2 {
		t.Errorf("Restore bool unquoted:\ngot:  %s\nwant: %s", restored2, want2)
	}
}

func TestRestore_ArraySentinel(t *testing.T) {
	const wantDNS = "dnsServers: ${DNS_SERVERS}"
	dnsEntry := SentinelEntry{Original: "${DNS_SERVERS}", Sentinel: `["__SENTARR_abcd1234__"]`, Type: typeArray}

	tests := []struct {
		name     string
		yamlText string
	}{
		{"block style", "dnsServers:\n    - __SENTARR_abcd1234__"},
		{"quoted block style", "dnsServers:\n  - \"__SENTARR_abcd1234__\""},
		{"flow style", `dnsServers: ["__SENTARR_abcd1234__"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restored := Restore(tt.yamlText, []SentinelEntry{dnsEntry})
			if restored != wantDNS {
				t.Errorf("Restore array %s:\ngot:  %s\nwant: %s", tt.name, restored, wantDNS)
			}
		})
	}
}

func TestRestoreNode_BlockScalarPreservesQuotes(t *testing.T) {
	// Sentinels embedded in a block scalar must not lose surrounding " characters
	// that are part of the shell script content.
	entries := []SentinelEntry{
		{Original: "${DEV%null}", Sentinel: "__SENTINEL_aaa__", Type: typeString},
		{Original: "${FALLBACK%null}", Sentinel: "__SENTINEL_bbb__", Type: typeString},
	}

	yamlText := "content: |\n  test -z \"__SENTINEL_aaa__\" && DEV=\"__SENTINEL_bbb__\"\n"
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &node); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	RestoreNode(&node, entries)

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		t.Fatalf("yaml.Encode: %v", err)
	}
	_ = enc.Close()

	want := "content: |\n  test -z \"${DEV%null}\" && DEV=\"${FALLBACK%null}\"\n"
	if got := buf.String(); got != want {
		t.Errorf("RestoreNode block scalar:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRestoreNode_FlowScalar(t *testing.T) {
	entries := []SentinelEntry{
		{Original: "${CLUSTER_NAME}", Sentinel: "__SENTINEL_aaa__", Type: typeString},
	}

	// After the Go round-trip, YAML-level quotes are stripped and the sentinel
	// appears bare as the node's Value.
	yamlText := "name: __SENTINEL_aaa__\n"
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &node); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	RestoreNode(&node, entries)

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		t.Fatalf("yaml.Encode: %v", err)
	}
	_ = enc.Close()

	if got := buf.String(); !strings.Contains(got, "${CLUSTER_NAME}") {
		t.Errorf("RestoreNode flow scalar: got %q, want ${CLUSTER_NAME} in output", got)
	}
}

func TestRestoreNode_IntExactMatch(t *testing.T) {
	entries := []SentinelEntry{
		{Original: "${REPLICAS:=3}", Sentinel: "99900001", Type: typeInt},
	}

	// Exact match: the sentinel integer must only be replaced when it is the
	// entire scalar value, not when it appears as a substring.
	yamlText := "replicas: 99900001\nport: 199900001\n"
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &node); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	RestoreNode(&node, entries)

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		t.Fatalf("yaml.Encode: %v", err)
	}
	_ = enc.Close()

	got := buf.String()
	if !strings.Contains(got, "${REPLICAS:=3}") {
		t.Errorf("RestoreNode int: want ${REPLICAS:=3} in output, got:\n%s", got)
	}
	if !strings.Contains(got, "199900001") {
		t.Errorf("RestoreNode int: port 199900001 should not be modified, got:\n%s", got)
	}
}

func TestRestore_NoEntries(t *testing.T) {
	input := "name: value"
	got := Restore(input, nil)
	if got != input {
		t.Errorf("Restore with no entries should return input unchanged")
	}
}

func TestGenerateSentinel(t *testing.T) {
	intCounter := 99900001
	// int sentinel
	s := generateSentinel("int", &intCounter, "")
	if s != "99900001" {
		t.Errorf("int sentinel = %q, want 99900001", s)
	}
	if intCounter != 99900002 {
		t.Errorf("intCounter should be incremented")
	}

	// int sentinel collision: text already contains the next value
	intCounter = 99900001
	s = generateSentinel("int", &intCounter, "99900001")
	if s != "99900002" {
		t.Errorf("int sentinel with collision = %q, want 99900002", s)
	}

	// int sentinel multi-collision: skip several occupied values.
	intCounter = 99900001
	s = generateSentinel("int", &intCounter, "99900001 and 99900002 and 99900003")
	if s != "99900004" {
		t.Errorf("int sentinel multi-collision = %q, want 99900004", s)
	}

	// bool sentinel
	s = generateSentinel("bool", &intCounter, "")
	if s != "true" { //nolint:goconst // explicit string test
		t.Errorf("bool sentinel = %q, want true", s)
	}

	// string sentinel
	s = generateSentinel("string", &intCounter, "")
	if !strings.HasPrefix(s, "__SENTINEL_") {
		t.Errorf("string sentinel = %q, should start with __SENTINEL_", s)
	}

	// array sentinel
	s = generateSentinel("array", &intCounter, "")
	if !strings.HasPrefix(s, `["__SENTARR_`) {
		t.Errorf("array sentinel = %q, should start with [\"__SENTARR_", s)
	}
}

func TestScanAndReplace_NoCollisionWithContent(t *testing.T) {
	// Input that contains strings resembling sentinel values.
	input := `name: __SENTINEL_deadbeef__
port: 99900001
dnsServers: ${DNS_SERVERS}
replicas: ${REPLICAS}
host: ${HOST}
`

	replaced, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}

	// Every sentinel must be unique and not collide with existing content.
	for _, e := range entries {
		// Count occurrences: the sentinel should appear exactly as many times
		// as the original expression appeared.
		origCount := strings.Count(input, e.Original)
		sentinelCount := strings.Count(replaced, e.Sentinel)
		if sentinelCount != origCount {
			t.Errorf("sentinel %q for %q appears %d times (expected %d) — possible collision",
				e.Sentinel, e.Original, sentinelCount, origCount)
		}
	}

	// Restore must recover the original exactly.
	restored := Restore(replaced, entries)
	if restored != input {
		t.Errorf("round-trip failed with collision-prone input.\nGot:\n%s\nWant:\n%s", restored, input)
	}
}

func TestScanAndReplace_SentinelsDoNotOverlap(t *testing.T) {
	// Multiple variables — no sentinel should be a substring of another.
	input := `spec:
  a: ${VAR_A}
  b: ${VAR_B}
  c: ${VAR_C}
  d: ${VAR_D}
  e: ${VAR_E}
`

	_, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}

	for i, ei := range entries {
		for j, ej := range entries {
			if i != j && strings.Contains(ei.Sentinel, ej.Sentinel) {
				t.Errorf("sentinel %q contains sentinel %q — overlap risk", ei.Sentinel, ej.Sentinel)
			}
		}
	}
}

func TestScanAndReplace_IntSentinelWordBoundary(t *testing.T) {
	// Verify integer sentinel restoration respects word boundaries.
	// If port sentinel is 99900001, it shouldn't match inside "999000010".
	input := `spec:
  port: ${PORT:=6443}
  name: ${NAME}
`

	replaced, entries, err := ScanAndReplace(input)
	if err != nil {
		t.Fatalf("ScanAndReplace: %v", err)
	}

	// Simulate a scenario where the sentinel appears as a substring of a larger number.
	// After conversion, append the sentinel digits to another number.
	var portEntry SentinelEntry
	for _, e := range entries {
		if e.Original == "${PORT:=6443}" {
			portEntry = e
			break
		}
	}

	mangled := strings.Replace(replaced, portEntry.Sentinel, portEntry.Sentinel+"0", 1)
	restored := Restore(mangled, entries)
	// The "0" appended should survive — the word-boundary regex should NOT match.
	if strings.Contains(restored, "${PORT:=6443}0") {
		t.Error("integer restore should respect word boundaries, not match partial numbers")
	}
}
