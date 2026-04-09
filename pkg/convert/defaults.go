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
	"gopkg.in/yaml.v3"
)

const tagNull = "!!null"

// pruneableFields maps resource kinds to field paths that should be pruned
// when they have default/zero values. These are fields added by ConvertTo
// that didn't exist in the source schema.
var pruneableFields = map[string]map[string][]string{
	KindProxmoxCluster: {
		"null":  {"zoneConfigs", "externalManagedControlPlane"},
		"false": {"externalManagedControlPlane"},
		"":      {"externalManagedControlPlane"},
	},
	KindProxmoxMachine: {
		"null": {"zoneConfigs"},
	},
	KindProxmoxMachineTemplate: {
		"null": {"zoneConfigs"},
	},
	KindProxmoxClusterTemplate: {
		"null":  {"zoneConfigs", "externalManagedControlPlane"},
		"false": {"externalManagedControlPlane"},
		"":      {"externalManagedControlPlane"},
	},
}

// PruneDefaults removes key-value pairs from the yaml.Node tree where the
// value matches a known default that should be omitted for the given resource kind.
func PruneDefaults(node *yaml.Node, resourceKind string) {
	pruneNullsDeep(node)
	pruneEmptyMappingsDeep(node)

	rules, ok := pruneableFields[resourceKind]
	if !ok {
		return
	}
	pruneByRules(node, rules)
}

// pruneNullsDeep removes all key-value pairs where the value is a null scalar,
// recursively through the tree.
func pruneNullsDeep(node *yaml.Node) {
	if node == nil {
		return
	}

	if node.Kind == yaml.MappingNode {
		filtered := make([]*yaml.Node, 0, len(node.Content))
		for i := 0; i+1 < len(node.Content); i += 2 {
			val := node.Content[i+1]
			if isNullScalar(val) {
				continue
			}
			pruneNullsDeep(val)
			filtered = append(filtered, node.Content[i], val)
		}
		node.Content = filtered
	}

	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			pruneNullsDeep(child)
		}
	}

	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			pruneNullsDeep(child)
		}
	}
}

// pruneEmptyMappingsDeep removes key-value pairs where the value is an empty
// mapping node (no keys), recursively. Runs bottom-up.
func pruneEmptyMappingsDeep(node *yaml.Node) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			pruneEmptyMappingsDeep(child)
		}
	case yaml.MappingNode:
		// First recurse into values.
		for i := 1; i < len(node.Content); i += 2 {
			pruneEmptyMappingsDeep(node.Content[i])
		}
		// Then prune empty mapping values.
		filtered := make([]*yaml.Node, 0, len(node.Content))
		for i := 0; i+1 < len(node.Content); i += 2 {
			val := node.Content[i+1]
			if val.Kind == yaml.MappingNode && len(val.Content) == 0 {
				continue
			}
			filtered = append(filtered, node.Content[i], val)
		}
		node.Content = filtered
	}
}

func isNullScalar(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == tagNull
}

func pruneByRules(node *yaml.Node, rules map[string][]string) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			pruneByRules(child, rules)
		}
	case yaml.MappingNode:
		filtered := make([]*yaml.Node, 0, len(node.Content))
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			if shouldPrune(key.Value, val, rules) {
				continue
			}
			pruneByRules(val, rules)
			filtered = append(filtered, key, val)
		}
		node.Content = filtered
	}
}

func shouldPrune(keyName string, val *yaml.Node, rules map[string][]string) bool {
	for defaultVal, fields := range rules {
		for _, f := range fields {
			if f == keyName && matchesDefault(val, defaultVal) {
				return true
			}
		}
	}
	return false
}

func matchesDefault(node *yaml.Node, defaultVal string) bool {
	if node.Kind != yaml.ScalarNode {
		return false
	}
	switch defaultVal {
	case "null":
		return node.Tag == tagNull
	case "":
		return node.Value == ""
	default:
		return node.Value == defaultVal
	}
}
