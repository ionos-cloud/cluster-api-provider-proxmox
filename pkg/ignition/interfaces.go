/*
Copyright 2024-2026 IONOS Cloud.

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

package ignition

// Renderer renders ignition data.
//
// It mirrors the cloudinit.Renderer contract, but Render returns a set of
// files keyed by name because an Ignition network configuration is emitted as
// multiple systemd-networkd unit files rather than a single document.
type Renderer interface {
	// Validate reports whether the data is structurally valid and renderable.
	// Render calls it, so callers need not invoke it separately.
	Validate() error
	Render() (map[string][]byte, error)
	Inspect() ([]byte, error)
}
