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
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
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

// Inspect returns a serialized copy of the NetworkData. This is useful when
// wanting to immutably inspect what goes into the renderer.
func (r *NetworkConfig) Inspect() ([]byte, error) {
	return json.Marshal(r.data.NetworkConfigData)
}

// Render returns rendered network-config.
func (r *NetworkConfig) Render() ([]byte, error) {
	// Validate inputs to template
	if err := r.validate(); err != nil {
		return nil, err
	}

	nc, err := render("network-config", networkConfigTpl, r.data)
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

func (r *NetworkConfig) validate() error {
	if len(r.data.NetworkConfigData) == 0 {
		return ErrMissingNetworkConfigData
	}

	// Tracks if a route already exists. A collision will return
	// confliction errConflictingMetrics.
	routeCollision := make(map[[32]byte]struct{})
	// Tracks whether any interface contributes a default gateway, either
	// explicitly via IPConfigs or implicitly via DHCP.
	// TODO: IPv6 slaac.
	hasGateway := false

	for _, d := range r.data.NetworkConfigData {
		if err := validateRoutes(d.Routes, &hasGateway, routeCollision); err != nil {
			return err
		}
		if err := validateFIBRules(d.FIBRules, d.Type == "vrf"); err != nil {
			return err
		}

		// This condition will require refactoring once more types of links
		// are added.
		if d.Type != "ethernet" {
			continue
		}

		if !d.DHCP4 && !d.DHCP6 && len(d.IPConfigs) == 0 {
			return ErrMissingIPAddress
		}

		if len(d.MacAddress) == 0 {
			return ErrMissingMacAddress
		}

		// DHCP may produce a default gateway. Skip further checks.
		if d.DHCP4 || d.DHCP6 {
			hasGateway = true
		}

		for _, c := range d.IPConfigs {
			// TODO: Probably useless
			if !c.IPAddress.IsValid() {
				return ErrMissingIPAddress
			}
		}
	}

	// If you end up here, please make an issue explaining how you need
	// a cluster without a default gateway. This is a valid usecase and
	// this check is merely an anti-footgun for regular users.
	// As a work around, set an invalid gateway which netlink can not
	// create.
	if !hasGateway {
		return ErrMissingGateway
	}

	return nil
}

func validateRoutes(routes []types.RoutingData, hasGateway *bool, routeCollisionMap map[[32]byte]struct{}) error {
	// No support for blackhole, etc.pp. Add iff you require this.
	for _, route := range routes {
		if !route.To.IsValid() {
			// Route without a target makes no sense.
			return ErrMalformedRoute
		}
		if route.To.Bits() == 0 && route.To.Addr().IsUnspecified() {
			*hasGateway = true
		}

		// A route is uniquely identified by its target subnet, metric and table.
		serialized := fmt.Sprintf("%s %d %d", route.To.String(), ptr.Deref(route.Metric, 0), ptr.Deref(route.Table, 0))
		routeID := sha256.Sum256([]byte(serialized))
		if _, exists := routeCollisionMap[routeID]; exists {
			return ErrConflictingMetrics
		}
		routeCollisionMap[routeID] = struct{}{}
	}
	return nil
}

func validateFIBRules(rules []types.FIBRuleData, isVrf bool) error {
	for _, rule := range rules {
		// We only support To/From and we require a table if we're not a vrf.
		if !rule.To.IsValid() && !rule.From.IsValid() {
			return ErrMalformedFIBRule
		}
		if ptr.Deref(rule.Table, 0) == 0 && !isVrf {
			return ErrMalformedFIBRule
		}
	}
	return nil
}
