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

import "fmt"

// Warning represents a structured warning emitted during conversion.
type Warning struct {
	File    string // filename or "<stdin>"
	Line    int    // 1-based line number in input (0 if unknown)
	Kind    string // "lost-comment", "passthrough", "lossy-conversion"
	Message string // human-readable description
	Old     string // quoted old field/value/comment (if applicable)
	New     string // quoted new field/value (if applicable)
}

// WarnFunc is a callback invoked immediately when a warning is produced.
type WarnFunc func(Warning)

// String formats the warning for stderr output.
func (w Warning) String() string {
	loc := w.File
	if w.Line > 0 {
		loc = fmt.Sprintf("%s:%d", w.File, w.Line)
	}

	s := fmt.Sprintf("WARNING [%s] %s: %s", loc, w.Kind, w.Message)
	if w.Old != "" {
		s += fmt.Sprintf("\n  old: %q", w.Old)
	}
	if w.New != "" {
		s += fmt.Sprintf("\n  new: %q", w.New)
	}
	return s
}
