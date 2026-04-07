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
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGraftComments_PreservesMatchingKeys(t *testing.T) {
	srcYAML := `# top comment
apiVersion: v1alpha1 # version comment
kind: Test
spec:
  # field comment
  name: foo # inline
`
	dstYAML := `apiVersion: v1alpha2
kind: Test
spec:
  name: bar
`

	var srcNode, dstNode yaml.Node
	if err := yaml.Unmarshal([]byte(srcYAML), &srcNode); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(dstYAML), &dstNode); err != nil {
		t.Fatal(err)
	}

	var warnings []Warning
	GraftComments(&srcNode, &dstNode, testfile, func(w Warning) {
		warnings = append(warnings, w)
	})
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(warnings), warnings)
	}

	out, err := yaml.Marshal(&dstNode)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if !containsSubstring(result, "# top comment") {
		t.Error("missing top comment")
	}
	if !containsSubstring(result, "# version comment") {
		t.Error("missing version comment")
	}
	if !containsSubstring(result, "# field comment") {
		t.Error("missing field comment")
	}
	if !containsSubstring(result, "# inline") {
		t.Error("missing inline comment")
	}
}

func TestGraftComments_LostCommentWarning(t *testing.T) {
	srcYAML := `apiVersion: v1alpha1
spec:
  name: foo
  removed: bar # important comment
`
	dstYAML := `apiVersion: v1alpha2
spec:
  name: foo
`

	var srcNode, dstNode yaml.Node
	if err := yaml.Unmarshal([]byte(srcYAML), &srcNode); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(dstYAML), &dstNode); err != nil {
		t.Fatal(err)
	}

	var warnings []Warning
	GraftComments(&srcNode, &dstNode, testfile, func(w Warning) {
		warnings = append(warnings, w)
	})
	if len(warnings) == 0 {
		t.Error("expected lost-comment warning")
	}

	found := false
	for _, w := range warnings {
		if w.Kind == "lost-comment" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning with kind=lost-comment")
	}
}

func TestGraftComments_SequenceByIndex(t *testing.T) {
	srcYAML := `items:
  - name: a # first item
  - name: b # second item
`
	dstYAML := `items:
  - name: a
  - name: b
`

	var srcNode, dstNode yaml.Node
	if err := yaml.Unmarshal([]byte(srcYAML), &srcNode); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(dstYAML), &dstNode); err != nil {
		t.Fatal(err)
	}

	var warnings []Warning
	GraftComments(&srcNode, &dstNode, testfile, func(w Warning) {
		warnings = append(warnings, w)
	})
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}

	out, err := yaml.Marshal(&dstNode)
	if err != nil {
		t.Fatal(err)
	}

	result := string(out)
	if !containsSubstring(result, "# first item") {
		t.Error("missing first item comment")
	}
	if !containsSubstring(result, "# second item") {
		t.Error("missing second item comment")
	}
}

func TestGraftComments_NilNodes(t *testing.T) {
	// Should not panic with nil nodes.
	GraftComments(nil, nil, testfile, func(w Warning) {
		t.Errorf("unexpected warning: %s", w)
	})
}

func TestGraftComments_TypeMismatch(t *testing.T) {
	srcYAML := `spec:
  items:
    - a
    - b
`
	dstYAML := `spec:
  items:
    key: value
`
	var srcNode, dstNode yaml.Node
	if err := yaml.Unmarshal([]byte(srcYAML), &srcNode); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(dstYAML), &dstNode); err != nil {
		t.Fatal(err)
	}

	// Should not panic when src is sequence but dst is mapping.
	GraftComments(&srcNode, &dstNode, testfile, noopWarn)
}

func TestGraftComments_SrcMappingDstNonMapping(t *testing.T) {
	srcYAML := `spec:
  name: foo
`
	// dst is a scalar where src expects a mapping
	dstYAML := `spec: bar
`
	var srcNode, dstNode yaml.Node
	if err := yaml.Unmarshal([]byte(srcYAML), &srcNode); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(dstYAML), &dstNode); err != nil {
		t.Fatal(err)
	}

	// Should not panic.
	GraftComments(&srcNode, &dstNode, testfile, noopWarn)
}

func TestGraftComments_FootComment(t *testing.T) {
	srcYAML := `apiVersion: v1
kind: Test
# foot comment
`
	dstYAML := `apiVersion: v1
kind: Test
`
	var srcNode, dstNode yaml.Node
	if err := yaml.Unmarshal([]byte(srcYAML), &srcNode); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(dstYAML), &dstNode); err != nil {
		t.Fatal(err)
	}

	GraftComments(&srcNode, &dstNode, testfile, noopWarn)

	out, _ := yaml.Marshal(&dstNode)
	result := string(out)
	if !containsSubstring(result, "foot comment") {
		t.Error("missing foot comment")
	}
}

func TestEmitFromNode_Nil(t *testing.T) {
	// Should not panic.
	emitFromNode(nil, ".test", testfile, func(_ Warning) {
		t.Error("unexpected warning on nil node")
	})
}

func TestCopyComments_EmptySrc(t *testing.T) {
	const existing = "existing"
	src := &yaml.Node{}
	dst := &yaml.Node{
		HeadComment: existing,
		LineComment: existing,
		FootComment: existing,
	}
	copyComments(src, dst)

	// Empty src comments should not overwrite existing dst comments.
	if dst.HeadComment != existing {
		t.Errorf("HeadComment overwritten: %q", dst.HeadComment)
	}
	if dst.LineComment != existing {
		t.Errorf("LineComment overwritten: %q", dst.LineComment)
	}
	if dst.FootComment != existing {
		t.Errorf("FootComment overwritten: %q", dst.FootComment)
	}
}

func TestEmitLostComments_DeepTree(t *testing.T) {
	// Test that emitLostComments walks children.
	key := &yaml.Node{
		Kind:        yaml.ScalarNode,
		Value:       "removedKey",
		LineComment: "# on key",
	}
	val := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "child", HeadComment: "# on child"},
			{Kind: yaml.ScalarNode, Value: "childval"},
		},
	}

	var warnings []Warning
	emitLostComments(key, val, ".removed", testfile, func(w Warning) {
		warnings = append(warnings, w)
	})

	if len(warnings) != 2 {
		t.Errorf("expected 2 lost-comment warnings, got %d", len(warnings))
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
