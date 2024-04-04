/*
Copyright 2023-2024 IONOS Cloud.

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
      {{- template "commonSettings" $element }}
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
          metric: {{ if eq .Name "eth0" }}100{{ else }}200{{ end }}
          via: {{ .Gateway }}
       {{- end }}
       {{- if .Gateway6 }}
        - to: '::/0'
          metric: {{ if eq .Name "eth0" }}100{{ else }}200{{ end }}
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
)

// NetworkConfig provides functionality to render machine network-config.
type NetworkConfig struct {
	data BaseCloudInitData
}

// NewNetworkConfig returns a new NetworkConfig object.
func NewNetworkConfig(configs []NetworkConfigData) *NetworkConfig {
	nc := new(NetworkConfig)
	nc.data = BaseCloudInitData{
		NetworkConfigData: configs,
	}
	return nc
}

// Render returns rendered network-config.
func (r *NetworkConfig) Render() ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}

	// render network-config
	return render("network-config", networkConfigTPl, r.data)
}

func (r *NetworkConfig) validate() error {
	if len(r.data.NetworkConfigData) == 0 {
		return ErrMissingNetworkConfigData
	}
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
	}
	return nil
}

func validRoutes(input []RoutingData) error {
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

func validFIBRules(input []FIBRuleData, isVrf bool) error {
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
