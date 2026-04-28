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
	"slices"

	"gopkg.in/yaml.v3"
)

// StripStatus removes the top-level "status" key from a YAML document node.
// If the status block contains any non-zero scalar values, it is kept and a
// warning is emitted instead.
func StripStatus(node *yaml.Node, filename string, warn WarnFunc) {
	if node == nil {
		return
	}

	// Unwrap document node.
	mapping := node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		mapping = node.Content[0]
	}
	if mapping.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		val := mapping.Content[i+1]

		if key.Value != "status" {
			continue
		}

		if hasNonZeroScalar(val) {
			warn(Warning{
				File:    filename,
				Kind:    "status",
				Message: "status block contains non-zero values and was kept; remove it manually if not needed",
			})
			return
		}

		// Remove the status key-value pair.
		mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
		return
	}
}

// hasNonZeroScalar returns true if any scalar descendant has a non-zero value.
func hasNonZeroScalar(node *yaml.Node) bool {
	if node == nil {
		return false
	}

	if node.Kind == yaml.ScalarNode {
		return !isZeroScalar(node)
	}

	if node.Kind == yaml.MappingNode {
		// Check values only (odd indices).
		for i := 1; i < len(node.Content); i += 2 {
			if hasNonZeroScalar(node.Content[i]) {
				return true
			}
		}
		return false
	}

	// SequenceNode, DocumentNode: check all children.
	return slices.ContainsFunc(node.Content, hasNonZeroScalar)
}

// isZeroScalar returns true for scalars that represent zero/empty/false/null.
func isZeroScalar(node *yaml.Node) bool {
	if node.Tag == tagNull {
		return true
	}
	switch node.Value {
	case "", "0", "false", "null", "~":
		return true
	}
	return false
}
