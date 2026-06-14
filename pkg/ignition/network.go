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

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"text/template"

	"github.com/pkg/errors"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

const (
	// networkConfigTplNetworkd is a Go template to generate systemd-networkd unit files
	// based on the data schema provided for network-config v2.
	networkConfigTplNetworkd = `
{{- define "dns" }}
  {{- if .DNSServers }}
    {{- range $dnsServer := .DNSServers }}
DNS={{ $dnsServer }}
    {{- end }}
  {{- end }}
{{- end }}

{{- define "rules" }}
  {{- $type := .Type }}
  {{- $table := .Table }}
  {{- range $rule := .FIBRules }}
[RoutingPolicyRule]
    {{- if $rule.To.IsValid }}
To={{ $rule.To }}
    {{- end }}
    {{- if $rule.From.IsValid }}
From={{ $rule.From }}
    {{- end }}
    {{- if $rule.Priority }}
Priority={{ $rule.Priority }}
    {{- end }}
    {{- if and (eq $type "vrf") (not $rule.Table) }}
Table={{ $table }}
    {{- else if $rule.Table }}
Table={{ $rule.Table }}
    {{- end }}
  {{- end }}
{{- end }}

{{- define "routes" }}
  {{- range $route := .Routes }}
[Route]
    {{- if $route.To.IsValid }}
Destination={{ $route.To }}
    {{- end }}
    {{- if $route.Via.IsValid }}
Gateway={{ $route.Via }}
    {{- end }}
    {{- if $route.Metric }}
Metric={{ $route.Metric }}
    {{- end }}
    {{- if $route.Table }}
Table={{ $route.Table }}
    {{- end }}
  {{- end }}
{{- end }}

{{- $element := . -}}
{{- $type := $element.Type -}}
{{- if eq $type "ethernet" -}}
[Match]
MACAddress={{ $element.MacAddress }}
  {{- if .LinkMTU }}
[Link]
MTUBytes={{ .LinkMTU }}
  {{- end }}
[Network]
  {{- if .VRF }}
VRF={{ .VRF }}
  {{- end }}
  {{- if and $element.DHCP4 $element.DHCP6 }}
DHCP=yes
  {{- else if $element.DHCP4 }}
DHCP=ipv4
  {{- else if $element.DHCP6 }}
DHCP=ipv6
  {{- end }}
  {{- template "dns" . }}
  {{- range $ipconfig := $element.IPConfigs }}
    {{- if .IPAddress }}
[Address]
Address={{ (.IPAddress).String }}
    {{- end }}
  {{- end }}
  {{- template "routes" . }}
  {{- template "rules" . }}
{{- end -}}
{{- if eq $type "vrf" -}}
[Match]
Name={{ $element.Name }}
  {{- template "routes" . }}
  {{- template "rules" . }}
{{- end }}
`

	netDevConfigTpl = `{{- $element := . -}}
{{- if eq $element.Type "vrf" -}}
[NetDev]
Name={{ $element.Name }}
Kind={{ $element.Type }}
[VRF]
Table={{ $element.Table }}
{{- end }}
`
)

// NetworkConfig provides functionality to render machine network-config into
// systemd-networkd unit files for Ignition.
//
// It embeds network.Network to inherit the shared, renderer-agnostic validation
// and layers its own ignition-specific checks on top via Validate.
type NetworkConfig struct {
	network.Network
}

var _ Renderer = (*NetworkConfig)(nil)

// NewNetworkConfig returns a new NetworkConfig object.
func NewNetworkConfig(configs []network.ConfigData) *NetworkConfig {
	return &NetworkConfig{network.Network{Devices: configs}}
}

// Inspect returns a serialized copy of the NetworkData. This is useful when
// wanting to immutably inspect what goes into the renderer.
func (r *NetworkConfig) Inspect() ([]byte, error) {
	return json.Marshal(r.Devices)
}

// Render returns the rendered systemd-networkd unit files keyed by filename.
func (r *NetworkConfig) Render() (map[string][]byte, error) {
	// Validate inputs to template.
	if err := r.Validate(); err != nil {
		return nil, err
	}

	return RenderNetworkConfigData(r.Devices)
}

// Validate runs the shared, renderer-agnostic validation (embedded
// network.Network). No further check is required because ignition
// implements the full stack exposed by networkConfigData.
func (r *NetworkConfig) Validate() error {
	return r.Network.Validate()
}

// RenderNetworkConfigData renders network-config data into systemd-networkd unit files.
func RenderNetworkConfigData(data []network.ConfigData) (map[string][]byte, error) {
	configs := make(map[string][]byte)

	// adjust VRFs
	adjustVrfs(data)

	// Add VRFs first so that they are created before the ethernet interfaces.
	n := 0
	for i, networkConfig := range data {
		// the []data.NetworkConfigData have types ethernet and vrf
		// we need to make sure to add vrf netdev first.
		// and that's why we use n to keep track of the vrf index.
		if networkConfig.Type == network.TypeVRF {
			config, err := render(fmt.Sprintf("%d-%s", i, networkConfig.Type), netDevConfigTpl, networkConfig)
			if err != nil {
				return nil, err
			}

			name := fmt.Sprintf("%02d-vrf%d.netdev", n, n)

			n++
			configs[name] = config
		}
	}

	for i, networkConfig := range data {
		config, err := render(fmt.Sprintf("%d-%s", i, networkConfig.Type), networkConfigTplNetworkd, networkConfig)
		if err != nil {
			return nil, err
		}

		name := "00-eth0.network"
		switch {
		case networkConfig.Type == network.TypeEthernet:
			name = fmt.Sprintf("%02d-eth%d.network", i, i)
		case networkConfig.Type == network.TypeVRF:
			name = fmt.Sprintf("%02d-vrf%d.network", i, i)
		}

		configs[name] = config
	}

	return configs, nil
}

func is6(addr string) bool {
	return netip.MustParsePrefix(addr).Addr().Is6()
}

func render(name string, tpl string, data network.ConfigData) ([]byte, error) {
	mt, err := template.New(name).Funcs(map[string]any{"is6": is6}).Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", name)
	}

	buffer := &bytes.Buffer{}
	if err = mt.Execute(buffer, data); err != nil {
		return nil, errors.Wrapf(err, "failed to render %s", name)
	}
	return buffer.Bytes(), nil
}

func adjustVrfs(data []network.ConfigData) {
	for i := range data {
		if data[i].Type != "vrf" {
			continue
		}
		// adjust VRFs, by adding the VRF name to each member ethernet interface.
		for _, child := range data[i].Children {
			for j := range data {
				if data[j].Name == child {
					data[j].VRF = data[i].Name
					break
				}
			}
		}
		// adjust VRF routes by adding the routing table to each member route.
		// This is to keep approximate expected behaviour with netplan v2, but
		// could use its own validation pass in the future.
		for j := range data[i].Routes {
			data[i].Routes[j].Table = data[i].Table
		}
	}
}
