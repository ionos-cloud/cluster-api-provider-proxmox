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

// Package convert provides YAML conversion for CAPMOX v1alpha1→v1alpha2
// and CAPI v1beta1→v1beta2 resources.
package convert

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Options configures the conversion.
type Options struct {
	Filename string   // filename for warning context (default: "<stdin>")
	Warn     WarnFunc // callback for warnings; if nil, warnings are discarded
	Indent   int      // output indentation; 0 = auto-detect from input (default 2)
}

// Convert processes a multi-document YAML text, converting each document
// through the appropriate converter pipeline. Warnings are emitted
// immediately via opts.Warn as they are discovered.
func Convert(input []byte, opts Options) ([]byte, error) {
	filename := opts.Filename
	if filename == "" {
		filename = "<stdin>"
	}
	warn := opts.Warn
	if warn == nil {
		warn = func(Warning) { /* dummy */ }
	}

	indent := opts.Indent
	if indent <= 0 {
		indent = detectIndent(input)
	}

	docs := splitYAMLDocuments(input)
	outputParts := make([][]byte, 0, len(docs))

	for _, doc := range docs {
		trimmed := strings.TrimSpace(string(doc))
		if trimmed == "" || trimmed == "---" {
			outputParts = append(outputParts, doc)
			continue
		}

		out, err := convertDocument(doc, filename, warn, indent)
		if err != nil {
			return nil, fmt.Errorf("converting document: %w", err)
		}

		outputParts = append(outputParts, out)
	}

	return joinYAMLDocuments(outputParts), nil
}

func convertDocument(rawDoc []byte, filename string, warn WarnFunc, indent int) ([]byte, error) {
	// Phase 1: Sentinel replacement.
	replaced, entries, err := ScanAndReplace(string(rawDoc))
	if err != nil {
		return nil, fmt.Errorf("sentinel replacement: %w", err)
	}

	sentinelDoc := []byte(replaced)

	// Phase 2: Detect resource type.
	id, converterType := DetectResource(sentinelDoc)

	var out []byte

	// Phase 3: Route to converter.
	switch converterType {
	case ConverterCAPMOX:
		out, err = ConvertCAPMOX(sentinelDoc, id, filename, indent, warn)
	case ConverterCAPI:
		out, err = ConvertCAPI(sentinelDoc, id, filename, indent, warn)
	case ConverterPassthrough:
		if id.APIVersion != "" || id.Kind != "" {
			warn(Warning{
				File:    filename,
				Kind:    "passthrough",
				Message: "resource not converted",
				Old:     fmt.Sprintf("apiVersion: %q, kind: %q", id.APIVersion, id.Kind),
			})
		}
		// Skip sentinel restoration for passthrough — return original.
		return rawDoc, nil
	}

	if err != nil {
		return nil, err
	}

	// Phase 5: Sentinel restoration.
	restored := Restore(string(out), entries)

	return []byte(restored), nil
}

// splitYAMLDocuments splits a multi-doc YAML by "---" separators,
// preserving the separator lines.
func splitYAMLDocuments(data []byte) [][]byte {
	var docs [][]byte
	lines := bytes.Split(data, []byte("\n"))
	current := make([][]byte, 0, len(lines))

	for _, line := range lines {
		if bytes.Equal(bytes.TrimSpace(line), []byte("---")) {
			if len(current) > 0 {
				doc := bytes.Join(current, []byte("\n"))
				docs = append(docs, append(doc, '\n'))
				current = nil
			}
			docs = append(docs, append(line, '\n'))
			continue
		}
		current = append(current, line)
	}

	if len(current) > 0 {
		docs = append(docs, bytes.Join(current, []byte("\n")))
	}

	return docs
}

func joinYAMLDocuments(parts [][]byte) []byte {
	return bytes.Join(parts, nil)
}

// marshalWithIndent encodes a yaml.Node using the specified indentation width.
func marshalWithIndent(node *yaml.Node, indent int) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(indent)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// detectIndent scans YAML input and returns the indentation width by finding
// the first line whose leading spaces form a common indent unit. Defaults to 2.
func detectIndent(data []byte) int {
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		trimmed := bytes.TrimLeft(line, " ")
		spaces := len(line) - len(trimmed)
		if spaces > 0 && spaces <= 8 {
			return spaces
		}
	}
	return 2
}
