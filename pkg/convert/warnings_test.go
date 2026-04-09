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

import "testing"

func TestWarningString(t *testing.T) {
	tests := []struct {
		name string
		w    Warning
		want string
	}{
		{
			name: "full warning with line number",
			w: Warning{
				File:    "template.yaml",
				Line:    42,
				Kind:    "lost-comment",
				Message: `comment on removed field "cloneSpec"`,
				Old:     "# SSH keys for node access",
			},
			want: "WARNING [template.yaml:42] lost-comment: comment on removed field \"cloneSpec\"\n  old: \"# SSH keys for node access\"",
		},
		{
			name: "passthrough warning without line",
			w: Warning{
				File:    "<stdin>",
				Line:    0,
				Kind:    "passthrough",
				Message: "resource not converted",
				Old:     `apiVersion: "v1", kind: "ConfigMap"`,
			},
			want: "WARNING [<stdin>] passthrough: resource not converted\n  old: \"apiVersion: \\\"v1\\\", kind: \\\"ConfigMap\\\"\"",
		},
		{
			name: "warning with old and new",
			w: Warning{
				File:    "template.yaml",
				Line:    15,
				Kind:    "lossy-conversion",
				Message: "field has no v1alpha2 equivalent",
				Old:     "spec.cloneSpec.sshAuthorizedKeys",
				New:     "KubeadmConfigTemplate users[].sshAuthorizedKeys",
			},
			want: "WARNING [template.yaml:15] lossy-conversion: field has no v1alpha2 equivalent\n  old: \"spec.cloneSpec.sshAuthorizedKeys\"\n  new: \"KubeadmConfigTemplate users[].sshAuthorizedKeys\"",
		},
		{
			name: "minimal warning",
			w: Warning{
				File:    "<stdin>",
				Kind:    "passthrough",
				Message: "unknown resource",
			},
			want: "WARNING [<stdin>] passthrough: unknown resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.w.String()
			if got != tt.want {
				t.Errorf("Warning.String() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
