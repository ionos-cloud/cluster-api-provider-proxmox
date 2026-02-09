/*
Copyright 2025 IONOS Cloud.

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

// Package types provides common types used in cloudinit & ignition.
package types

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
	Gateway   string
	Metric    *int32
	Default   bool
}

// RoutingData stores routing configuration.
type RoutingData = infrav1.RouteSpec

// FIBRuleData stores forward information base rules (routing policies).
type FIBRuleData = infrav1.RoutingPolicySpec
