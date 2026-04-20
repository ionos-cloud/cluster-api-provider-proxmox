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
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Sentinel type constants.
const (
	typeString = "string"
	typeInt    = "int"
	typeBool   = "bool"
	typeArray  = "array"
)

// SentinelEntry maps an envsubst expression to its sentinel replacement.
type SentinelEntry struct {
	Original string // "${NUM_SOCKETS:=2}"
	Sentinel string // "99900001"
	Type     string // typeString, typeInt, typeBool, typeArray
}

var envsubstRe = regexp.MustCompile(`\$\{[^}]+\}`)

// integerKeys is the set of YAML keys known to take integer values.
var integerKeys = map[string]bool{
	"replicas":     true,
	"numSockets":   true,
	"numCores":     true,
	"memoryMiB":    true,
	"templateID":   true,
	"sizeGb":       true,
	"diskSize":     true,
	"port":         true,
	"templateVMID": true,
	"machineCount": true,
	"prefix":       true,
}

// arrayKeys is the set of YAML keys known to take array values.
var arrayKeys = map[string]bool{
	"dnsServers":        true,
	"addresses":         true,
	"allowedNodes":      true,
	"cidrBlocks":        true,
	"sshAuthorizedKeys": true,
}

// ScanAndReplace finds all ${...} expressions in yamlText, replaces them with
// type-appropriate sentinel values, and returns the modified text plus the mapping.
func ScanAndReplace(yamlText string) (string, []SentinelEntry, error) {
	matches := envsubstRe.FindAllString(yamlText, -1)
	if len(matches) == 0 {
		return yamlText, nil, nil
	}

	// Deduplicate while preserving order.
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}

	intCounter := 99900001
	entries := make([]SentinelEntry, 0, len(unique))

	for _, expr := range unique {
		typ := inferType(expr, yamlText)
		sentinel := generateSentinel(typ, &intCounter, yamlText)
		entries = append(entries, SentinelEntry{
			Original: expr,
			Sentinel: sentinel,
			Type:     typ,
		})
	}

	replaced := yamlText
	for _, e := range entries {
		replaced = strings.ReplaceAll(replaced, e.Original, e.Sentinel)
	}

	return replaced, entries, nil
}

// Restore reverses sentinel replacement in raw YAML text. Used for array
// sentinels (structural text replacement) and as a fallback. Prefer RestoreNode
// for string/bool/int sentinels — it handles block scalar content correctly.
func Restore(yamlText string, entries []SentinelEntry) string {
	result := yamlText

	for _, e := range entries {
		switch e.Type {
		case typeString, typeBool:
			result = strings.ReplaceAll(result, e.Sentinel, e.Original)
		case typeInt:
			// Quoted integer sentinels: "99900001" → ${VAR} (remove quotes).
			result = strings.ReplaceAll(result, `"`+e.Sentinel+`"`, e.Original)
			// Unquoted integer sentinels with word boundary.
			re := regexp.MustCompile(`\b` + regexp.QuoteMeta(e.Sentinel) + `\b`)
			result = re.ReplaceAllLiteralString(result, e.Original)
		case typeArray:
			// Array sentinels: the sentinel was `["__SENTARR_hex__"]` in the input,
			// which after Go round-trip becomes a YAML sequence with one element.
			// We need to find patterns like:
			//   key:\n<indent>- __SENTARR_hex__   →   key: ${ORIGINAL}
			//   key:\n<indent>- "__SENTARR_hex__"  →   key: ${ORIGINAL}
			innerSentinel := extractArraySentinelInner(e.Sentinel)
			if innerSentinel != "" {
				// Match multi-line: "key:\n    - sentinel" patterns.
				arrayRe := regexp.MustCompile(`(?m)(:\n\s*)- "?` + regexp.QuoteMeta(innerSentinel) + `"?`)
				result = arrayRe.ReplaceAllLiteralString(result, ": "+e.Original)
			}
			// Also try direct replacement for flow-style arrays that survived.
			result = strings.ReplaceAll(result, e.Sentinel, e.Original)
		}
	}

	return result
}

// inferType determines the Go type a sentinel must satisfy for the given expression.
func inferType(expr, yamlText string) string {
	defaultVal := extractDefault(expr)
	if defaultVal != "" {
		if _, err := strconv.Atoi(defaultVal); err == nil {
			return typeInt
		}
		if defaultVal == "true" || defaultVal == "false" { //nolint:goconst // literal values, not type constants
			return typeBool
		}
		if strings.HasPrefix(defaultVal, "[") {
			return typeArray
		}
		return typeString
	}

	// Check known-fields table by scanning the line containing the expression.
	key := extractYAMLKey(expr, yamlText)
	if integerKeys[key] {
		return typeInt
	}
	if arrayKeys[key] && isWholeValue(expr, yamlText) {
		return typeArray
	}

	return typeString
}

// extractDefault parses "${VAR:=default}" and returns "default", or "" if no default.
func extractDefault(expr string) string {
	// Strip ${ and }
	inner := expr[2 : len(expr)-1]
	_, after, ok := strings.Cut(inner, ":=")
	if !ok {
		return ""
	}
	return after
}

// extractYAMLKey finds the YAML key on the line containing expr.
func extractYAMLKey(expr, yamlText string) string {
	idx := strings.Index(yamlText, expr)
	if idx < 0 {
		return ""
	}

	// Find start of line.
	lineStart := strings.LastIndex(yamlText[:idx], "\n") + 1
	line := yamlText[lineStart:idx]

	// The key is the trimmed text before the colon.
	line = strings.TrimSpace(line)
	if before, _, ok := strings.Cut(line, ":"); ok {
		return strings.TrimSpace(before)
	}
	return ""
}

// isWholeValue checks if the expression is the sole value after the colon on its line
// (not embedded in brackets or concatenated with other text).
func isWholeValue(expr, yamlText string) bool {
	idx := strings.Index(yamlText, expr)
	if idx < 0 {
		return false
	}
	lineStart := strings.LastIndex(yamlText[:idx], "\n") + 1
	lineEnd := strings.Index(yamlText[idx:], "\n")
	if lineEnd < 0 {
		lineEnd = len(yamlText)
	} else {
		lineEnd += idx
	}
	line := yamlText[lineStart:lineEnd]
	_, after, ok := strings.Cut(line, ":")
	if !ok {
		return false
	}
	value := strings.TrimSpace(after)
	return value == expr
}

// generateUnique calls gen repeatedly until a value is found that does not
// appear in yamlText, avoiding collisions with existing content.
func generateUnique(yamlText string, gen func() string) string {
	for {
		s := gen()
		if !strings.Contains(yamlText, s) {
			return s
		}
	}
}

// generateSentinel creates a sentinel value for the given type.
func generateSentinel(typ string, intCounter *int, yamlText string) string {
	switch typ {
	case typeInt:
		return generateUnique(yamlText, func() string {
			s := strconv.Itoa(*intCounter)
			*intCounter++
			return s
		})
	case typeBool:
		return "true"
	case typeArray:
		return generateUnique(yamlText, randomArraySentinel)
	default: // string
		return generateUnique(yamlText, randomHexSentinel)
	}
}

// extractArraySentinelInner extracts the inner sentinel string from an array sentinel
// like `["__SENTARR_abc__"]` → `__SENTARR_abc__`.
func extractArraySentinelInner(sentinel string) string {
	s := strings.TrimPrefix(sentinel, `["`)
	s = strings.TrimSuffix(s, `"]`)
	if s != sentinel {
		return s
	}
	return ""
}

// randomArraySentinel generates a sentinel for array fields — a YAML flow sequence
// with a single sentinel element that survives Go struct round-trips.
func randomArraySentinel() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf(`["__SENTARR_%x__"]`, b)
}

// randomHexSentinel generates a sentinel like __SENTINEL_a1b2c3d4__.
func randomHexSentinel() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("__SENTINEL_%x__", b)
}

// RestoreNode replaces sentinels in a yaml.Node tree by operating on each
// scalar's Value directly. This correctly handles block scalars where " chars
// are literal content — a plain substring replace leaves them intact.
// Array sentinels (which expand to sequence nodes) are left for Restore.
func RestoreNode(node *yaml.Node, entries []SentinelEntry) {
	if node == nil || len(entries) == 0 {
		return
	}
	if node.Kind == yaml.ScalarNode {
		for _, e := range entries {
			switch e.Type {
			case typeInt:
				// Exact match: avoid partial replacement inside other numbers.
				if node.Value == e.Sentinel {
					node.Value = e.Original
					node.Tag = "" // drop !!int so go-yaml re-infers from the restored string
					node.Style = 0
				}
			case typeArray:
				// Array sentinels expand to sequence nodes; handled by Restore.
			default: // typeString, typeBool
				node.Value = strings.ReplaceAll(node.Value, e.Sentinel, e.Original)
			}
		}
	}
	for _, child := range node.Content {
		RestoreNode(child, entries)
	}
}
