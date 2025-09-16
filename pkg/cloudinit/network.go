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

	"k8s.io/utils/ptr"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
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
    {{- if or .IPConfigs .Routes }}
      routes:
      {{- range $ipconfig := .IPConfigs }}
      {{- if .Gateway }}
       {{- if .Gateway }}
        {{- if is6 .IPAddress }}
        - to: '::/0'
        {{- else }}
        - to: 0.0.0.0/0
        {{- end -}}
          {{- if .Metric }}
          metric: {{ .Metric }}
          {{- end }}
          via: {{ .Gateway }}
       {{- end }}
      {{- end -}}
      {{- end -}}
      {{- if .Routes }}
        {{- range $index, $route := .Routes }}
        - {
        {{- if $route.To }} "to": "{{$route.To}}", {{ end -}}
        {{- if $route.Via }} "via": "{{$route.Via}}", {{ end -}}
        {{- if $route.Metric }} "metric": {{$route.Metric}}, {{ end -}}
        {{- if $route.Table }} "table": {{$route.Table}}, {{ end -}} }
        {{- end -}}
        {{- end -}}
    {{- end -}}
{{- end -}}

{{- define "ipAddresses" }}
    {{- if .IPConfigs }}
      addresses:
        {{- range $ipconfig := .IPConfigs }}
        - {{ .IPAddress }}
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
type NetworkConfig struct {
	data BaseCloudInitData
}

// NewNetworkConfig returns a new NetworkConfig object.
func NewNetworkConfig(configs []types.NetworkConfigData) *NetworkConfig {
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
	return render("network-config", networkConfigTpl, r.data)
}

func (r *NetworkConfig) validate() error {
	if len(r.data.NetworkConfigData) == 0 {
		return ErrMissingNetworkConfigData
	}
	// TODO: Fix validation
	metrics := make(map[uint32]*struct {
		ipv4 bool
		ipv6 bool
	})

	// for i, d := range r.data.NetworkConfigData {
	for _, d := range r.data.NetworkConfigData {
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

		if !d.DHCP4 && !d.DHCP6 && len(d.IPConfigs) == 0 {
			return ErrMissingIPAddress
		}

		if d.MacAddress == "" {
			return ErrMissingMacAddress
		}

		for _, c := range d.IPConfigs {
			var is6 bool
			var err error

			if !d.DHCP4 || !d.DHCP6 {
				is6, err = validIPAddress(c.IPAddress)
				if err != nil {
					return err
				}
				if c.Gateway == "" /*&& i == 0*/ {
					return ErrMissingGateway
				}
			}

			if c.Metric != nil {
				if _, exists := metrics[*c.Metric]; !exists {
					metrics[*c.Metric] = new(struct {
						ipv4 bool
						ipv6 bool
					})
				}
				if !is6 && metrics[*c.Metric].ipv4 {
					return ErrConflictingMetrics
				}
				if is6 && metrics[*c.Metric].ipv6 {
					return ErrConflictingMetrics
				}
				if !is6 {
					metrics[*c.Metric].ipv4 = true
				} else {
					metrics[*c.Metric].ipv6 = true
				}
			}
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
		if ptr.Deref(route.To, "") != "default" {
			// An IP address is a valid route (implicit smallest subnet)
			_, errPrefix := netip.ParsePrefix(ptr.Deref(route.To, ""))
			_, errAddr := netip.ParseAddr(ptr.Deref(route.To, ""))
			if errPrefix != nil && errAddr != nil {
				return ErrMalformedRoute
			}
		}
		if ptr.Deref(route.Via, "") != "" {
			_, err := netip.ParseAddr(ptr.Deref(route.Via, ""))
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
		if (ptr.Deref(rule.To, "") == "" && ptr.Deref(rule.From, "") == "") || (ptr.Deref(rule.Table, 0) == 0 && !isVrf) {
			return ErrMalformedFIBRule
		}
		if ptr.Deref(rule.To, "") != "" {
			_, errPrefix := netip.ParsePrefix(ptr.Deref(rule.To, ""))
			_, errAddr := netip.ParseAddr(ptr.Deref(rule.To, ""))
			if errPrefix != nil && errAddr != nil {
				return ErrMalformedFIBRule
			}
		}
		if ptr.Deref(rule.From, "") != "" {
			_, errPrefix := netip.ParsePrefix(ptr.Deref(rule.From, ""))
			_, errAddr := netip.ParseAddr(ptr.Deref(rule.From, ""))
			if errPrefix != nil && errAddr != nil {
				return ErrMalformedFIBRule
			}
		}
	}
	return nil
}

func validIPAddress(input string) (bool, error) {
	if input == "" {
		return false, ErrMissingIPAddress
	}
	p, err := netip.ParsePrefix(input)
	if err != nil {
		return false, ErrMalformedIPAddress
	}
	return p.Addr().Is6(), nil
}
