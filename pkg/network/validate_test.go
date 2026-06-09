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

package network

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func defaultRoute(metric int32) RoutingData {
	return RoutingData{
		To:     netip.MustParsePrefix("0.0.0.0/0"),
		Via:    netip.MustParseAddr("10.0.0.1"),
		Metric: ptr.To(metric),
	}
}

func TestNetwork_Validate(t *testing.T) {
	cases := map[string]struct {
		devices []NetworkConfigData
		err     error
	}{
		"no devices": {
			devices: nil,
			err:     ErrMissingNetworkConfigData,
		},
		"no gateway anywhere": {
			devices: []NetworkConfigData{
				{Type: "ethernet", Name: "eth0", MacAddress: "x", IPConfigs: []IPConfig{{IPAddress: netip.MustParsePrefix("10.0.0.2/24")}}},
			},
			err: ErrMissingGateway,
		},
		"dhcp satisfies gateway": {
			devices: []NetworkConfigData{
				{Type: "ethernet", Name: "eth0", DHCP4: true},
			},
		},
		"single default route is valid": {
			devices: []NetworkConfigData{
				{Type: "ethernet", Name: "eth0", Routes: []RoutingData{defaultRoute(100)}},
			},
		},
		"route without target is malformed": {
			devices: []NetworkConfigData{
				{Type: "ethernet", Name: "eth0", Routes: []RoutingData{{Metric: ptr.To[int32](100)}}},
			},
			err: ErrMalformedRoute,
		},
		"duplicate route in main table collides": {
			devices: []NetworkConfigData{
				{Type: "ethernet", Name: "eth0", Routes: []RoutingData{defaultRoute(100)}},
				{Type: "ethernet", Name: "eth1", Routes: []RoutingData{defaultRoute(100)}},
			},
			err: ErrConflictingMetrics,
		},
		"identical routes in different VRFs do not collide": {
			devices: []NetworkConfigData{
				// Each NIC carries an identical default route, but each is
				// attached to a different VRF, so the routes land in different
				// tables and must not be flagged as conflicting.
				{Type: "ethernet", Name: "eth0", Routes: []RoutingData{defaultRoute(100)}},
				{Type: "ethernet", Name: "eth1", Routes: []RoutingData{defaultRoute(100)}},
				{Type: "vrf", Name: "vrf-a", Table: ptr.To[int32](100), Children: []string{"eth0"}},
				{Type: "vrf", Name: "vrf-b", Table: ptr.To[int32](200), Children: []string{"eth1"}},
			},
		},
		"identical routes in the same VRF collide": {
			devices: []NetworkConfigData{
				{Type: "ethernet", Name: "eth0", Routes: []RoutingData{defaultRoute(100)}},
				{Type: "ethernet", Name: "eth1", Routes: []RoutingData{defaultRoute(100)}},
				{Type: "vrf", Name: "vrf-a", Table: ptr.To[int32](100), Children: []string{"eth0", "eth1"}},
			},
			err: ErrConflictingMetrics,
		},
		"explicit per-route table overrides device table": {
			devices: []NetworkConfigData{
				// Two NICs in the same VRF (table 100), but one route is pinned
				// to table 200 explicitly, so they no longer collide.
				{Type: "ethernet", Name: "eth0", Routes: []RoutingData{defaultRoute(100)}},
				{Type: "ethernet", Name: "eth1", Routes: []RoutingData{{
					To:     netip.MustParsePrefix("0.0.0.0/0"),
					Via:    netip.MustParseAddr("10.0.0.1"),
					Metric: ptr.To[int32](100),
					Table:  ptr.To[int32](200),
				}}},
				{Type: "vrf", Name: "vrf-a", Table: ptr.To[int32](100), Children: []string{"eth0", "eth1"}},
			},
		},
		"ethernet FIB rule requires a table": {
			devices: []NetworkConfigData{
				{Type: "ethernet", Name: "eth0", DHCP4: true, FIBRules: []FIBRuleData{{
					From: netip.MustParsePrefix("10.0.0.0/8"),
				}}},
			},
			err: ErrMalformedFIBRule,
		},
		"FIB rule mixing address families is malformed": {
			devices: []NetworkConfigData{
				// A rule's source and destination must share one family; the
				// kernel rejects a mixed-family rule (net/core/fib_rules.c).
				{Type: "ethernet", Name: "eth0", DHCP4: true, FIBRules: []FIBRuleData{{
					To:    netip.MustParsePrefix("2001:db8::/64"),
					From:  netip.MustParsePrefix("10.0.0.0/8"),
					Table: ptr.To[int32](500),
				}}},
			},
			err: ErrMalformedFIBRule,
		},
		"FIB rule with matching families is valid": {
			devices: []NetworkConfigData{
				{Type: "ethernet", Name: "eth0", DHCP4: true, FIBRules: []FIBRuleData{{
					To:    netip.MustParsePrefix("2001:db8::/64"),
					From:  netip.MustParsePrefix("2001:db8:1::/64"),
					Table: ptr.To[int32](500),
				}}},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			n := &Network{Devices: tc.devices}
			err := n.Validate()
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
		})
	}
}
