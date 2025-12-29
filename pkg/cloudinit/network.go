/*
Copyright 2023-2025 IONOS Cloud.

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
	"net/netip"
	"strings"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
)

const (
	/* network-config template. */
	networkConfigTPl = `network:
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
    {{- if $element.Interfaces }}
      interfaces:
      {{- range $element.Interfaces }}
        - {{ . }}
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
        {{- if $rule.To }} "to": "{{$rule.To}}", {{ end -}}
        {{- if $rule.From }} "from": "{{$rule.From}}", {{ end -}}
        {{- if $rule.Priority }} "priority": {{$rule.Priority}}, {{ end -}}
        {{- if $rule.Table }} "table": {{$rule.Table}}, {{ end -}} }
      {{- end }}
    {{- end }}
{{- end -}}

{{- define "routes" }}
    {{- if or .Gateway .Gateway6 }}
      routes:
       {{- if .Gateway }}
        - to: 0.0.0.0/0
          {{- if .Metric }}
          metric: {{ .Metric }}
          {{- end }}
          via: {{ .Gateway }}
       {{- end }}
       {{- if .Gateway6 }}
        - to: '::/0'
          {{- if .Metric6 }}
          metric: {{ .Metric6 }}
          {{- end }}
          via: '{{ .Gateway6 }}'
       {{- end }}
    {{- else }}
      {{- if .Routes }}
      routes:
      {{- end -}}
    {{- end -}}
    {{- range $index, $route := .Routes }}
        - {
        {{- if $route.To }} "to": "{{$route.To}}", {{ end -}}
        {{- if $route.Via }} "via": "{{$route.Via}}", {{ end -}}
        {{- if $route.Metric }} "metric": {{$route.Metric}}, {{ end -}}
        {{- if $route.Table }} "table": {{$route.Table}}, {{ end -}} }
    {{- end -}}
{{- end -}}

{{- define "ipAddresses" }}
    {{- if or .IPAddress .IPV6Address }}
      addresses:
      {{- if .IPAddress }}
        - {{ .IPAddress }}
      {{- end }}
      {{- if .IPV6Address }}
        - '{{ .IPV6Address }}'
      {{- end }}
    {{- end }}
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
type NetworkConfig struct {
	data   BaseCloudInitData
	format infrav1alpha1.CloudInitNetworkConfigFormat
}

// NewNetworkConfig returns a new NetworkConfig object.
// The format parameter specifies the cloud-init network-config format to use.
// If empty, defaults to netplan format.
func NewNetworkConfig(configs []types.NetworkConfigData, format infrav1alpha1.CloudInitNetworkConfigFormat) *NetworkConfig {
	nc := new(NetworkConfig)
	nc.data = BaseCloudInitData{
		NetworkConfigData: configs,
	}
	// Default to netplan format if not specified
	if format == "" {
		format = infrav1alpha1.CloudInitNetworkConfigFormatNetplan
	}
	nc.format = format
	return nc
}

// Render returns rendered network-config.
func (r *NetworkConfig) Render() ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}

	// render network-config using netplan template
	rendered, err := render("network-config", networkConfigTPl, r.data)
	if err != nil {
		return nil, err
	}

	// For nocloud format, convert by stripping "network:" wrapper and dedenting
	if r.format == infrav1alpha1.CloudInitNetworkConfigFormatNoCloud {
		// Validate that no netplan-only features are used
		if err := r.validateNoCloudCompatibility(); err != nil {
			return nil, err
		}
		return convertToNoCloud(rendered), nil
	}

	return rendered, nil
}

// convertToNoCloud transforms netplan format to nocloud format by removing
// the "network:" wrapper and dedenting the content by 2 spaces.
func convertToNoCloud(netplanConfig []byte) []byte {
	lines := strings.Split(string(netplanConfig), "\n")
	var result []string
	for i, line := range lines {
		if i == 0 && line == "network:" {
			continue // Skip "network:" line
		}
		if len(line) >= 2 && line[:2] == "  " {
			result = append(result, line[2:]) // Remove 2 leading spaces
		} else {
			result = append(result, line)
		}
	}
	return []byte(strings.Join(result, "\n"))
}

// validateNoCloudCompatibility checks that no netplan-only features are used
// when nocloud format is requested. VRFs and routing-policy are not supported
// by cloud-init's network-config v2 format.
func (r *NetworkConfig) validateNoCloudCompatibility() error {
	for _, d := range r.data.NetworkConfigData {
		// VRFs are not supported by cloud-init v2
		if d.Type == "vrf" {
			return ErrNoCloudUnsupportedFeature
		}
		// routing-policy (FIBRules) is not supported by cloud-init v2
		if len(d.FIBRules) > 0 {
			return ErrNoCloudUnsupportedFeature
		}
	}
	return nil
}

func (r *NetworkConfig) validate() error {
	if len(r.data.NetworkConfigData) == 0 {
		return ErrMissingNetworkConfigData
	}
	metrics := make(map[uint32]*struct {
		ipv4 bool
		ipv6 bool
	})

	for i, d := range r.data.NetworkConfigData {
		// TODO: refactor this when network configuration is unified
		if d.Type != "ethernet" {
			err := validRoutes(d.Routes)
			if err != nil {
				return err
			}
			err = validFIBRules(d.FIBRules, true)
			if err != nil {
				return err
			}
			continue
		}

		if !d.DHCP4 && !d.DHCP6 && len(d.IPAddress) == 0 && len(d.IPV6Address) == 0 {
			return ErrMissingIPAddress
		}

		if d.MacAddress == "" {
			return ErrMissingMacAddress
		}

		if !d.DHCP4 && len(d.IPAddress) > 0 {
			err := validIPAddress(d.IPAddress)
			if err != nil {
				return err
			}
			if d.Gateway == "" && i == 0 {
				return ErrMissingGateway
			}
		}

		if !d.DHCP6 && len(d.IPV6Address) > 0 {
			err6 := validIPAddress(d.IPV6Address)
			if err6 != nil {
				return err6
			}
			if d.Gateway6 == "" && i == 0 {
				return ErrMissingGateway
			}
		}
		if d.Metric != nil {
			if _, exists := metrics[*d.Metric]; !exists {
				metrics[*d.Metric] = new(struct {
					ipv4 bool
					ipv6 bool
				})
			}
			if metrics[*d.Metric].ipv4 {
				return ErrConflictingMetrics
			}
			metrics[*d.Metric].ipv4 = true
		}
		if d.Metric6 != nil {
			if _, exists := metrics[*d.Metric6]; !exists {
				metrics[*d.Metric6] = new(struct {
					ipv4 bool
					ipv6 bool
				})
			}

			if metrics[*d.Metric6].ipv6 {
				return ErrConflictingMetrics
			}
			metrics[*d.Metric6].ipv6 = true
		}
	}
	return nil
}

func validRoutes(input []types.RoutingData) error {
	if len(input) == 0 {
		return nil
	}
	// No support for blackhole, etc.pp. Add iff you require this.
	for _, route := range input {
		if route.To != "default" {
			// An IP address is a valid route (implicit smallest subnet)
			_, errPrefix := netip.ParsePrefix(route.To)
			_, errAddr := netip.ParseAddr(route.To)
			if errPrefix != nil && errAddr != nil {
				return ErrMalformedRoute
			}
		}
		if route.Via != "" {
			_, err := netip.ParseAddr(route.Via)
			if err != nil {
				return ErrMalformedRoute
			}
		}
	}
	return nil
}

func validFIBRules(input []types.FIBRuleData, isVrf bool) error {
	if len(input) == 0 {
		return nil
	}

	for _, rule := range input {
		// We only support To/From and we require a table if we're not a vrf
		if (rule.To == "" && rule.From == "") || (rule.Table == 0 && !isVrf) {
			return ErrMalformedFIBRule
		}
		if rule.To != "" {
			_, errPrefix := netip.ParsePrefix(rule.To)
			_, errAddr := netip.ParseAddr(rule.To)
			if errPrefix != nil && errAddr != nil {
				return ErrMalformedFIBRule
			}
		}
		if rule.From != "" {
			_, errPrefix := netip.ParsePrefix(rule.From)
			_, errAddr := netip.ParseAddr(rule.From)
			if errPrefix != nil && errAddr != nil {
				return ErrMalformedFIBRule
			}
		}
	}
	return nil
}

func validIPAddress(input string) error {
	if input == "" {
		return ErrMissingIPAddress
	}
	_, err := netip.ParsePrefix(input)
	if err != nil {
		return ErrMalformedIPAddress
	}
	return nil
}
