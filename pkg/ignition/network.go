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
	"fmt"
	"net/netip"
	"text/template"

	"github.com/pkg/errors"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
)

const (
	networkTypeEthernet = "ethernet"
	networkTypeVRF      = "vrf"

	// networkConfigTplNetworkd is a Go template to generate systemd-networkd unit files
	// based on the data schema provided for network-config v2.
	networkConfigTplNetworkd = `{{- $element := . -}}
{{- $type := $element.Type -}}
{{ if eq $type "ethernet" -}}
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
{{- range $ipconfig := $element.IPConfigs -}}
{{ if .IPAddress }}
[Address]
Address={{ (.IPAddress).String }}
{{- end -}}
{{- if .Gateway }}
[Route]
{{- if ((.IPAddress).Addr).Is6 }}
Destination=::/0
{{- else }}
Destination=0.0.0.0/0
{{- end }}
Gateway={{ .Gateway }}
{{- if .Metric }}
Metric={{ .Metric }}
{{- end }}
{{- end }}
{{- end }}
{{ template "routes" . -}}
{{ template "rules" . -}}
{{- end -}}
{{- if eq $type "vrf" -}}
[Match]
Name={{ $element.Name }}
{{ template "routes" . -}}
{{- template "rules" . -}}
{{- end -}}
{{- define "dns" }}
{{- if .DNSServers }}
{{- range $dnsServer := .DNSServers }}
DNS={{ $dnsServer }}
{{- end }}
{{- end }}
{{- end }}
{{- define "rules" }}
{{- if .FIBRules -}}
{{- $type := .Type }}
{{- $table := .Table }}
{{- range $index, $rule := .FIBRules -}}
[RoutingPolicyRule]
{{ if $rule.To }}To={{$rule.To}}{{- end }}
{{ if $rule.From }}From={{$rule.From}}{{- end }}
{{ if $rule.Priority }}Priority={{$rule.Priority}}{{- end }}
{{ if and (eq $type "vrf") (not $rule.Table) }}Table={{ $table }}
{{ else }}{{ if $rule.Table }}Table={{$rule.Table}}
{{ end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- define "routes" }}
{{- if .Routes }}
{{- range $index, $route := .Routes -}}
[Route]
{{ if $route.To }}Destination={{$route.To}}{{- end }}
{{ if $route.Via }}Gateway={{$route.Via}}{{- end }}
{{ if $route.Metric }}Metric={{$route.Metric}}{{- end }}
{{ if $route.Table }}Table={{$route.Table}}{{- end }}
{{- end -}}
{{- end -}}
{{- end -}}
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

// RenderNetworkConfigData renders network-config data into systemd-networkd unit files.
func RenderNetworkConfigData(data []types.NetworkConfigData) (map[string][]byte, error) {
	configs := make(map[string][]byte)

	// adjust VRFs
	adjustVrfs(data)

	// Add VRFs first so that they are created before the ethernet interfaces.
	n := 0
	for i, networkConfig := range data {
		// the []data.NetworkConfigData have types ethernet and vrf
		// we need to make sure to add vrf netdev first.
		// and that's why we use n to keep track of the vrf index.
		if networkConfig.Type == networkTypeVRF {
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
		case networkConfig.Type == networkTypeEthernet:
			name = fmt.Sprintf("%02d-eth%d.network", i, i)
		case networkConfig.Type == networkTypeVRF:
			name = fmt.Sprintf("%02d-vrf%d.network", i, i)
		}

		configs[name] = config
	}

	return configs, nil
}

func is6(addr string) bool {
	return netip.MustParsePrefix(addr).Addr().Is6()
}

func render(name string, tpl string, data types.NetworkConfigData) ([]byte, error) {
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

func adjustVrfs(data []types.NetworkConfigData) {
	// adjust VRFs, by adding the VRF name to the ethernet interface.
	for i, networkConfig := range data {
		if networkConfig.Type == "ethernet" {
			for _, vrf := range data {
				if vrf.Type == "vrf" {
					for _, iface := range vrf.Interfaces {
						if iface == networkConfig.Name {
							data[i].VRF = vrf.Name
						}
					}
				}
			}
		}
	}
}
