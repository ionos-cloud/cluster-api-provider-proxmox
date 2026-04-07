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

// GraftComments copies comments from src yaml.Node tree to dst yaml.Node tree,
// matching nodes by structure. Emits warnings immediately via warn for any
// comments in src that could not be grafted to dst.
func GraftComments(src, dst *yaml.Node, filename string, warn WarnFunc) {
	graftNode(src, dst, "", filename, warn)
}

func graftNode(src, dst *yaml.Node, path, filename string, warn WarnFunc) {
	if src == nil || dst == nil {
		return
	}

	// Copy comments from src to dst at this level.
	copyComments(src, dst)

	switch src.Kind {
	case yaml.DocumentNode:
		if dst.Kind == yaml.DocumentNode && len(src.Content) > 0 && len(dst.Content) > 0 {
			graftNode(src.Content[0], dst.Content[0], path, filename, warn)
		}

	case yaml.MappingNode:
		if dst.Kind != yaml.MappingNode {
			return
		}
		graftMapping(src, dst, path, filename, warn)

	case yaml.SequenceNode:
		if dst.Kind != yaml.SequenceNode {
			return
		}
		graftSequence(src, dst, path, filename, warn)

	case yaml.ScalarNode:
		// Already copied comments above.
	}
}

func graftMapping(src, dst *yaml.Node, path, filename string, warn WarnFunc) {
	// Build index of dst keys for fast lookup.
	dstKeys := make(map[string]int) // key name → index of value node in dst.Content
	for i := 0; i+1 < len(dst.Content); i += 2 {
		dstKeys[dst.Content[i].Value] = i
	}

	for i := 0; i+1 < len(src.Content); i += 2 {
		srcKey := src.Content[i]
		srcVal := src.Content[i+1]
		keyName := srcKey.Value
		childPath := path + "." + keyName

		if dstIdx, ok := dstKeys[keyName]; ok {
			dstKey := dst.Content[dstIdx]
			dstVal := dst.Content[dstIdx+1]
			copyComments(srcKey, dstKey)
			graftNode(srcVal, dstVal, childPath, filename, warn)
		} else {
			// Key exists in src but not dst — emit lost comments immediately.
			emitLostComments(srcKey, srcVal, childPath, filename, warn)
		}
	}
}

func graftSequence(src, dst *yaml.Node, path, filename string, warn WarnFunc) {
	for i := 0; i < len(src.Content) && i < len(dst.Content); i++ {
		childPath := path + "[]"
		graftNode(src.Content[i], dst.Content[i], childPath, filename, warn)
	}
}

func copyComments(src, dst *yaml.Node) {
	if src.HeadComment != "" {
		dst.HeadComment = src.HeadComment
	}
	if src.LineComment != "" {
		dst.LineComment = src.LineComment
	}
	if src.FootComment != "" {
		dst.FootComment = src.FootComment
	}
}

// emitLostComments walks a node tree that has no match in dst and emits
// a warning immediately for each comment found.
func emitLostComments(key, val *yaml.Node, path, filename string, warn WarnFunc) {
	emitFromNode(key, path, filename, warn)
	emitFromNode(val, path, filename, warn)
}

func emitFromNode(node *yaml.Node, path, filename string, warn WarnFunc) {
	if node == nil {
		return
	}
	for _, comment := range []string{node.HeadComment, node.LineComment, node.FootComment} {
		if comment != "" {
			warn(Warning{
				File:    filename,
				Line:    node.Line,
				Kind:    "lost-comment",
				Message: "comment on removed field " + path,
				Old:     comment,
			})
		}
	}
	for _, child := range node.Content {
		emitFromNode(child, path, filename, warn)
	}
}
