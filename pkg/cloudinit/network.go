/*
Copyright 2023-2026 IONOS Cloud.

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

package cloudinit

import (
	"bytes"
	"encoding/json"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

// EmptyNetworkV1 is an empty network-config for version 1.
const EmptyNetworkV1 = `version: 1
config: []`

// NetworkConfig provides functionality to render machine network-config.
//
// It embeds network.Network to inherit the shared, renderer-agnostic validation
// and layers its own cloud-init-specific checks on top via Validate.
type NetworkConfig struct {
	network.Network
}

// NewNetworkConfig returns a new NetworkConfig object.
func NewNetworkConfig(configs []network.ConfigData) *NetworkConfig {
	return &NetworkConfig{network.Network{Devices: configs}}
}

// Inspect returns a serialized copy of the NetworkData. This is useful when
// wanting to immutably inspect what goes into the renderer.
func (r *NetworkConfig) Inspect() ([]byte, error) {
	return json.Marshal(r.Devices)
}

// Render returns rendered network-config.
//
// The netplan v2 config is built as a typed structure and marshalled to YAML,
// so the output is valid by construction (no post-hoc parse check) and special
// characters in names/addresses are quoted correctly.
func (r *NetworkConfig) Render() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	// netplan examples and the previous renderer use two-space indentation.
	enc.SetIndent(2)
	if err := enc.Encode(buildNetplanConfig(r.Devices)); err != nil {
		return nil, errors.Wrap(err, "failed to marshal network-config")
	}
	if err := enc.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to flush network-config")
	}

	// Drop the trailing newline the encoder appends, to match the previous
	// renderer's output.
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Validate runs the shared, renderer-agnostic validation (embedded
// network.Network). No further renderer specific validation is required
// (netplan implements every feature in networkConfigData).
func (r *NetworkConfig) Validate() error {
	return r.Network.Validate()
}
