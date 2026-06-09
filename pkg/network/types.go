/*
Copyright 2025-2026 IONOS Cloud.

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

// Package network provides common types used in cloudinit & ignition.
package network

import (
	"net/netip"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// NetworkConfigData is used to render network-config.
type NetworkConfigData struct {
	ProxName   infrav1.NetName // Device name in Proxmox
	MacAddress string
	DHCP4      bool
	DHCP6      bool
	IPConfigs  []IPConfig
	DNSServers []string
	Type       string
	Name       string
	Interfaces []string // Interfaces controlled by this one.
	Table      int32    // linux routing table number for VRF.
	Routes     []RoutingData
	FIBRules   []FIBRuleData // Forwarding information block for routing.
	LinkMTU    infrav1.MTU   // linux network device MTU
	VRF        string        // linux VRF name // only used in networkd config.
}

// IPConfig stores IP configuration.
type IPConfig struct {
	IPAddress netip.Prefix
	Default   bool
}

// RoutingData stores a single route.
//
// The address family is encoded in To once it has been parsed (the
// RouteSpec Is6 field only disambiguates placeholder targets such as
// "default"/"all" at parse time and is not carried here). Use To.IsValid()
// to check for a 'zero address'.
type RoutingData struct {
	To     netip.Prefix
	Table  *int32
	Via    netip.Addr
	Metric *int32
}

// FIBRuleData stores a single Linux FIB (forwarding information base) rule.
//
// As with RoutingData, the address family is encoded in To/From after parsing.
// Use To.IsValid()/From.IsValid() to check for a 'zero address'.
type FIBRuleData struct {
	To       netip.Prefix
	Table    *int32
	From     netip.Prefix
	Priority *int64
}
