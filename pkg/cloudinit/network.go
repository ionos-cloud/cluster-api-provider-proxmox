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
	"encoding/json"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

const (
	/* network-config template. */
	networkConfigTpl = `network:
  version: 2
  renderer: networkd
  ethernets:
{{- range $index, $element := .NetworkConfigData }}
  {{- $type := $element.Type }}
  {{- if eq $type "ethernet" }}
    {{ $element.Name }}:
      match:
        macaddress: {{ $element.MacAddress }}
      {{- template "commonSettings" $element }}
  {{- end -}}
{{- end -}}
{{- $vrf := 0 -}}
{{- range $index, $element := .NetworkConfigData }}
  {{- if eq $element.Type "vrf" }}
  {{- if eq $vrf 0 }}
  vrfs:
  {{- $vrf = 1 }}
  {{- end }}
    {{$element.Name}}:
      table: {{ $element.Table }}
    {{- template "routes" . }}
    {{- template "rules" . }}
    {{- $interfaces := $element.Children }}
    {{- if $interfaces }}
      interfaces:
      {{- range $interfaces }}
        - '{{ . }}'
      {{- end -}}
    {{- end -}}
  {{- end }}
{{- end -}}

  {{- define "dns" }}
    {{- if .DNSServers }}
      nameservers:
        addresses:
        {{- range .DNSServers }}
          - '{{ . }}'
        {{- end -}}
    {{- end -}}
  {{- end -}}

{{- define "dhcp" }}
      dhcp4: {{ if .DHCP4 }}true{{ else }}false{{ end }}
      dhcp6: {{ if .DHCP6 }}true{{ else }}false{{ end }}
{{- end -}}

{{- define "rules" }}
    {{- if .FIBRules }}
      routing-policy:
      {{- range $index, $rule := .FIBRules }}
        - {
        {{- if $rule.To.IsValid }} "to": "{{$rule.To}}", {{ end -}}
        {{- if $rule.From.IsValid }} "from": "{{$rule.From}}", {{ end -}}
        {{- if $rule.Priority }} "priority": {{$rule.Priority}}, {{ end -}}
        {{- if $rule.Table }} "table": {{$rule.Table}}, {{ end -}} }
      {{- end }}
    {{- end }}
{{- end -}}

{{- define "routes" }}
    {{- if .Routes }}
      routes:
        {{- range $index, $route := .Routes }}
        - {
          {{- if $route.To.IsValid }} "to": "{{$route.To}}", {{ end -}}
          {{- if $route.Via.IsValid }} "via": "{{$route.Via}}", {{ end -}}
          {{- if $route.Metric }} "metric": {{$route.Metric}}, {{ end -}}
          {{- if $route.Table }} "table": {{$route.Table}}, {{ end -}} }
        {{- end -}}
    {{- end -}}
{{- end -}}

{{- define "ipAddresses" }}
    {{- if .IPConfigs }}
      addresses:
        {{- range $ipconfig := .IPConfigs }}
        - '{{ (.IPAddress).String }}'
        {{- end }}
    {{- end -}}
{{- end -}}

{{- define "mtu" }}
    {{- if .LinkMTU }}
      mtu: {{ .LinkMTU }}
    {{- end -}}
{{- end -}}

{{- define "commonSettings" }}
    {{- template "dhcp" . }}
    {{- template "ipAddresses" . }}
    {{- template "routes" . }}
    {{- template "rules" . }}
    {{- template "dns" . }}
    {{- template "mtu" . }}
{{- end -}}
`
	// EmptyNetworkV1 is an empty network-config for version 1.
	EmptyNetworkV1 = `version: 1
config: []`
)

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
func (r *NetworkConfig) Render() ([]byte, error) {
	// Validate inputs to template
	if err := r.Validate(); err != nil {
		return nil, err
	}

	nc, err := render("network-config", networkConfigTpl, BaseCloudInitData{NetworkConfigData: r.Devices})
	if err != nil {
		return nil, err
	}

	// Check YAML render to be valid
	var unused interface{}
	err = yaml.Unmarshal(nc, &unused)
	if err != nil {
		return nil, errors.Wrap(err,
			"Template produced invalid YAML. Please file a bug at: "+
				"https://github.com/ionos-cloud/cluster-api-provider-proxmox/")
	}

	return nc, nil
}

// Validate runs the shared, renderer-agnostic validation (embedded
// network.Network). No further renderer specific validation is required
// (netplan implements every feature in networkConfigData).
func (r *NetworkConfig) Validate() error {
	return r.Network.Validate()
}
