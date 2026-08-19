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

package cloudinit

import (
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

// The types below model the subset of the netplan v2 schema that CAPMOX emits.
// They are marshalled to YAML directly (see NetworkConfig.Render) instead of
// being rendered through a text template, so the output is valid by
// construction and special characters are quoted correctly.
//
// Field order within each struct matches the desired YAML key order, and
// pointer / slice fields use `omitempty` so that a key is emitted only when the
// corresponding ConfigData value is set — mirroring the previous template's
// conditionals.

type netplanConfig struct {
	Network netplanNetwork `yaml:"network"`
}

type netplanNetwork struct {
	Version   int                        `yaml:"version"`
	Renderer  string                     `yaml:"renderer"`
	Ethernets map[string]netplanEthernet `yaml:"ethernets,omitempty"`
	VRFs      map[string]netplanVRF      `yaml:"vrfs,omitempty"`
}

type netplanEthernet struct {
	Match *netplanMatch `yaml:"match,omitempty"`
	// dhcp4/dhcp6 are always emitted (netplan treats an absent key as false, but
	// the previous renderer emitted both explicitly, so we keep that behaviour).
	DHCP4         bool                `yaml:"dhcp4"`
	DHCP6         bool                `yaml:"dhcp6"`
	Addresses     []string            `yaml:"addresses,omitempty"`
	Routes        []netplanRoute      `yaml:"routes,omitempty"`
	RoutingPolicy []netplanRule       `yaml:"routing-policy,omitempty"`
	Nameservers   *netplanNameservers `yaml:"nameservers,omitempty"`
	MTU           *int32              `yaml:"mtu,omitempty"`
}

type netplanMatch struct {
	MACAddress string `yaml:"macaddress"`
}

type netplanNameservers struct {
	Addresses []string `yaml:"addresses"`
}

type netplanRoute struct {
	To     string `yaml:"to,omitempty"`
	Via    string `yaml:"via,omitempty"`
	Metric *int32 `yaml:"metric,omitempty"`
	Table  *int32 `yaml:"table,omitempty"`
}

type netplanRule struct {
	To       string `yaml:"to,omitempty"`
	From     string `yaml:"from,omitempty"`
	Priority *int64 `yaml:"priority,omitempty"`
	Table    *int32 `yaml:"table,omitempty"`
}

type netplanVRF struct {
	Table         *int32         `yaml:"table,omitempty"`
	Routes        []netplanRoute `yaml:"routes,omitempty"`
	RoutingPolicy []netplanRule  `yaml:"routing-policy,omitempty"`
	Interfaces    []string       `yaml:"interfaces,omitempty"`
}

// buildNetplanConfig maps the renderer-agnostic network model to the netplan v2
// schema. Devices are keyed by name; yaml.v3 marshals maps with sorted keys, so
// the output is deterministic.
func buildNetplanConfig(devices []network.ConfigData) netplanConfig {
	cfg := netplanConfig{
		Network: netplanNetwork{
			Version:  2,
			Renderer: "networkd",
		},
	}

	for i := range devices {
		d := devices[i]
		switch d.Type {
		case network.TypeEthernet:
			if cfg.Network.Ethernets == nil {
				cfg.Network.Ethernets = map[string]netplanEthernet{}
			}
			cfg.Network.Ethernets[d.Name] = netplanEthernet{
				Match:         &netplanMatch{MACAddress: d.MacAddress},
				DHCP4:         d.DHCP4,
				DHCP6:         d.DHCP6,
				Addresses:     netplanAddresses(d.IPConfigs),
				Routes:        netplanRoutes(d.Routes),
				RoutingPolicy: netplanRules(d.FIBRules),
				Nameservers:   netplanNameserversFor(d.DNSServers),
				MTU:           (*int32)(d.LinkMTU),
			}
		case network.TypeVRF:
			if cfg.Network.VRFs == nil {
				cfg.Network.VRFs = map[string]netplanVRF{}
			}
			cfg.Network.VRFs[d.Name] = netplanVRF{
				Table:         d.Table,
				Routes:        netplanRoutes(d.Routes),
				RoutingPolicy: netplanRules(d.FIBRules),
				Interfaces:    d.Children,
			}
		}
	}

	return cfg
}

func netplanAddresses(configs []network.IPConfig) []string {
	if len(configs) == 0 {
		return nil
	}
	out := make([]string, 0, len(configs))
	for _, c := range configs {
		out = append(out, c.IPAddress.String())
	}
	return out
}

func netplanNameserversFor(dnsServers []string) *netplanNameservers {
	if len(dnsServers) == 0 {
		return nil
	}
	return &netplanNameservers{Addresses: dnsServers}
}

func netplanRoutes(routes []network.RoutingData) []netplanRoute {
	if len(routes) == 0 {
		return nil
	}
	out := make([]netplanRoute, 0, len(routes))
	for _, r := range routes {
		nr := netplanRoute{Metric: r.Metric, Table: r.Table}
		if r.To.IsValid() {
			nr.To = r.To.String()
		}
		if r.Via.IsValid() {
			nr.Via = r.Via.String()
		}
		out = append(out, nr)
	}
	return out
}

func netplanRules(rules []network.FIBRuleData) []netplanRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]netplanRule, 0, len(rules))
	for _, r := range rules {
		nr := netplanRule{Priority: r.Priority, Table: r.Table}
		if r.To.IsValid() {
			nr.To = r.To.String()
		}
		if r.From.IsValid() {
			nr.From = r.From.String()
		}
		out = append(out, nr)
	}
	return out
}
