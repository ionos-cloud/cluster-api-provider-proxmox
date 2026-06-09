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

package vmservice

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

func TestExtractUUID(t *testing.T) {
	type match struct {
		test     string
		expected string
	}

	goodstrings := []match{
		{"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b", "7dd9b137-6a3c-4661-a4fa-375075e1776b"},
		{"foo=bar,uuid=71A5f8b4-5d30-43a3-b902-242393ad80b5,baz=quux", "71a5f8b4-5d30-43a3-b902-242393ad80b5"},
		{",uuid=e80432e2-2b5c-4539-af97-852aaa7e84d7", "e80432e2-2b5c-4539-af97-852aaa7e84d7"},
	}

	badstrings := []string{
		"fuuid=4594e7d0-3aa6-4235-95b2-6b0018192b0a",
		"uuid=123abc-def",
		"uid=8a689fee-1653-40ec-a4bf-e88b8dabacd6",
		"uuid",
		"",
		"foobar",
	}

	for _, m := range goodstrings {
		require.Equal(t, m.expected, extractUUID(m.test))
	}

	for _, s := range badstrings {
		require.Equal(t, "", extractUUID(s))
	}
}

func TestExtractNetworkModel(t *testing.T) {
	type match struct {
		test     string
		expected string
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "virtio"},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "virtio"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "virtio"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,foo", "virtio"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,foo=bar", "virtio"},
		{"foo=bar,e1000=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,foo=bar", "e1000"},
		{"foo=bar,e1000=a6:23:64:4d:84:Cb,bridge=vmbr1,mtu=9000,foo=bar", "e1000"},
	}

	badstrings := []string{
		"bridge=vmbr1",
		"virtio=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"a6:23:64:4d:84:Cb",
		"=",
		"",
	}

	for _, m := range goodstrings {
		model := extractNetworkModel(m.test)
		require.Equal(t, m.expected, model)
	}

	for _, s := range badstrings {
		model := extractNetworkModel(s)
		require.Empty(t, model)
	}
}

func TestExtractNetworkBridge(t *testing.T) {
	type match struct {
		test     string
		expected string
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "vmbr1"},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,foo", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,foo=bar", "vmbr1"},
	}

	badstrings := []string{
		"virtio=",
		"bridge=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"",
	}

	for _, m := range goodstrings {
		bridge := extractNetworkBridge(m.test)
		require.Equal(t, m.expected, bridge)
	}

	for _, s := range badstrings {
		bridge := extractNetworkBridge(s)
		require.Equal(t, "unknown", bridge)
	}
}

func TestExtractNetworkMTU(t *testing.T) {
	type match struct {
		test     string
		expected int32
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", 1500},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", 1500},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", 1500},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,foo", 9000},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,foo=bar", 9000},
	}

	badstrings := []string{
		"virtio=",
		"bridge=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"",
		"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9999999999", // int32 overflow
	}

	for _, m := range goodstrings {
		mtu := extractNetworkMTU(m.test)
		require.Equal(t, m.expected, mtu)
	}

	for _, s := range badstrings {
		mtu := extractNetworkMTU(s)
		require.Equal(t, int32(0), mtu)
	}
}

func TestShouldUpdateNetworkDevices_NoNetworkConfig(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	machineScope.ProxmoxMachine.Spec.Network = nil

	require.False(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_MissingDefaultDeviceOnVM(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{Bridge: ptr.To("vmbr1"), Model: ptr.To("virtio")}},
	}
	machineScope.SetVirtualMachine(newStoppedVM())

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_DefaultDeviceNeedsUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{Bridge: ptr.To("vmbr1"), Model: ptr.To("virtio")}},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_MissingAdditionalDeviceOnVM(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Name:   "net1",
				Bridge: ptr.To("vmbr1"),
				Model:  ptr.To("virtio"),
			},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_AdditionalDeviceNeedsUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Name:   "net1",
				Bridge: ptr.To("vmbr1"),
				Model:  ptr.To("virtio"),
			},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("", "virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_NoUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{
			{
				Bridge: ptr.To("vmbr0"),
				Model:  ptr.To("virtio"),
				MTU:    ptr.To[int32](1500),
			},
			{
				Name:   "net1",
				Bridge: ptr.To("vmbr1"),
				Model:  ptr.To("virtio"),
				MTU:    ptr.To[int32](1500),
			},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CD,bridge=vmbr0,mtu=1500", "virtio=A6:23:64:4D:84:CD,bridge=vmbr1,mtu=1500"))

	require.False(t, shouldUpdateNetworkDevices(machineScope))
}

func TestExtractNetworkVLAN(t *testing.T) {
	type match struct {
		test     string
		expected int32
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,tag=100", 100},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,tag=100", 100},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,tag=100", 100},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,tag=200,foo", 200},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,tag=200,foo=bar", 200},
	}

	badstrings := []string{
		"virtio=",
		"bridge=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"",
		"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,tag=9999999999", // int32 overflow
	}

	for _, m := range goodstrings {
		vlan := extractNetworkVLAN(m.test)
		require.Equal(t, m.expected, vlan)
	}

	for _, s := range badstrings {
		vlan := extractNetworkVLAN(s)
		require.Equal(t, int32(0), vlan)
	}
}

func TestShouldUpdateNetworkDevices_VLANChanged(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{Bridge: ptr.To("vmbr0"), Model: ptr.To("virtio"), VLAN: ptr.To(int32(100))}},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0,tag=101"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestExtractNetworkQueue(t *testing.T) {
	type match struct {
		test     string
		expected int32
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,queues=4", 4},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,queues=4", 4},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,queues=4", 4},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,tag=100,queues=8,foo=bar", 8},
	}

	badstrings := []string{
		"virtio=",
		"bridge=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"",
		"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,queues=65536", // uint16 overflow
	}

	for _, m := range goodstrings {
		queue := extractNetworkQueue(m.test)
		require.Equal(t, m.expected, queue)
	}

	for _, s := range badstrings {
		queue := extractNetworkQueue(s)
		require.Equal(t, int32(0), queue)
	}
}

func TestFormatNetworkDevice(t *testing.T) {
	require.Equal(t, "virtio,bridge=vmbr0", formatNetworkDevice("virtio", "vmbr0", nil, nil, nil))
	require.Equal(t, "virtio,bridge=vmbr0,mtu=1500", formatNetworkDevice("virtio", "vmbr0", ptr.To(int32(1500)), nil, nil))
	require.Equal(t, "virtio,bridge=vmbr0,tag=100", formatNetworkDevice("virtio", "vmbr0", nil, ptr.To(int32(100)), nil))
	require.Equal(t, "virtio,bridge=vmbr0,queues=4", formatNetworkDevice("virtio", "vmbr0", nil, nil, ptr.To(int32(4))))
	require.Equal(t, "virtio,bridge=vmbr0,mtu=1500,tag=100,queues=4", formatNetworkDevice("virtio", "vmbr0", ptr.To(int32(1500)), ptr.To(int32(100)), ptr.To(int32(4))))
}

func TestExtractMACAddress(t *testing.T) {
	type match struct {
		test     string
		expected string
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1", "A6:23:64:4D:84:CB"},
		{"e1000=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "A6:23:64:4D:84:CB"},
	}

	badstrings := []string{
		"bridge=vmbr1",
		"virtio=,bridge=vmbr1",
		"",
	}

	for _, m := range goodstrings {
		mac := extractMACAddress(m.test)
		require.Equal(t, m.expected, mac)
	}

	for _, s := range badstrings {
		mac := extractMACAddress(s)
		require.Empty(t, mac)
	}
}

func TestParseRouteTarget(t *testing.T) {
	cases := []struct {
		name   string
		s      *string
		is6    *bool
		expect netip.Prefix
		err    bool
	}{
		// nil / empty string -> zero prefix (unset)
		{name: "nil input", s: nil, expect: netip.Prefix{}},
		{name: "empty string", s: ptr.To(""), expect: netip.Prefix{}},

		// "default" / "all" aliases -> IPv4 unspecified by default
		{name: "default -> ipv4", s: ptr.To("default"), expect: netip.MustParsePrefix("0.0.0.0/0")},
		{name: "all -> ipv4", s: ptr.To("all"), expect: netip.MustParsePrefix("0.0.0.0/0")},
		{name: "default is6=false -> ipv4", s: ptr.To("default"), is6: ptr.To(false), expect: netip.MustParsePrefix("0.0.0.0/0")},
		{name: "default is6=true -> ipv6", s: ptr.To("default"), is6: ptr.To(true), expect: netip.MustParsePrefix("::/0")},
		{name: "all is6=true -> ipv6", s: ptr.To("all"), is6: ptr.To(true), expect: netip.MustParsePrefix("::/0")},

		// CIDR prefixes pass through normalised
		{name: "ipv4 prefix", s: ptr.To("10.0.0.0/8"), expect: netip.MustParsePrefix("10.0.0.0/8")},
		{name: "ipv6 prefix", s: ptr.To("::/0"), expect: netip.MustParsePrefix("::/0")},
		{name: "host prefix /32", s: ptr.To("192.168.1.1/32"), expect: netip.MustParsePrefix("192.168.1.1/32")},
		{name: "host prefix /128", s: ptr.To("2001:db8::1/128"), expect: netip.MustParsePrefix("2001:db8::1/128")},

		// Bare IP addresses mapped to /32 or /128
		{name: "bare ipv4", s: ptr.To("10.0.0.1"), expect: netip.PrefixFrom(netip.MustParseAddr("10.0.0.1"), 32)},
		{name: "bare ipv6", s: ptr.To("2001:db8::1"), expect: netip.PrefixFrom(netip.MustParseAddr("2001:db8::1"), 128)},

		// Invalid -> error
		{name: "garbage", s: ptr.To("not-an-ip"), err: true},
		{name: "partial", s: ptr.To("10.0.0"), err: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRouteTarget(tc.s, tc.is6)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expect, got)
		})
	}
}

func TestParseVia(t *testing.T) {
	cases := []struct {
		name   string
		s      *string
		expect netip.Addr
		err    bool
	}{
		// nil / empty -> zero address (unset)
		{name: "nil", s: nil, expect: netip.Addr{}},
		{name: "empty", s: ptr.To(""), expect: netip.Addr{}},

		// Valid bare addresses
		{name: "ipv4", s: ptr.To("10.0.0.1"), expect: netip.MustParseAddr("10.0.0.1")},
		{name: "ipv6", s: ptr.To("2001:db8::1"), expect: netip.MustParseAddr("2001:db8::1")},

		// Prefixes and placeholders are rejected (Via must be a bare address)
		{name: "prefix rejected", s: ptr.To("10.0.0.0/8"), err: true},
		{name: "default rejected", s: ptr.To("default"), err: true},
		{name: "all rejected", s: ptr.To("all"), err: true},
		{name: "garbage", s: ptr.To("not-an-ip"), err: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseVia(tc.s)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expect, got)
		})
	}
}

func TestToRoutingData(t *testing.T) {
	table100 := ptr.To[int32](100)
	metric50 := ptr.To[int32](50)

	cases := []struct {
		name   string
		specs  []infrav1.RouteSpec
		expect []network.RoutingData
		err    bool
	}{
		{
			name:   "nil input maps to empty slice",
			specs:  nil,
			expect: []network.RoutingData{},
		},
		{
			name: "default gateway ipv4",
			specs: []infrav1.RouteSpec{
				{To: ptr.To("default"), Via: ptr.To("10.0.0.1"), Metric: metric50},
			},
			expect: []network.RoutingData{{
				To:     netip.MustParsePrefix("0.0.0.0/0"),
				Via:    netip.MustParseAddr("10.0.0.1"),
				Metric: metric50,
			}},
		},
		{
			name: "default gateway ipv6 via is6",
			specs: []infrav1.RouteSpec{
				{To: ptr.To("default"), Is6: ptr.To(true), Via: ptr.To("2001:db8::1")},
			},
			expect: []network.RoutingData{{
				To:  netip.MustParsePrefix("::/0"),
				Via: netip.MustParseAddr("2001:db8::1"),
			}},
		},
		{
			name: "default gateway ipv6 family derived from via without is6",
			specs: []infrav1.RouteSpec{
				{To: ptr.To("default"), Via: ptr.To("2001:db8::1")},
			},
			expect: []network.RoutingData{{
				To:  netip.MustParsePrefix("::/0"),
				Via: netip.MustParseAddr("2001:db8::1"),
			}},
		},
		{
			name: "default gateway ipv4 family derived from via without is6",
			specs: []infrav1.RouteSpec{
				{To: ptr.To("all"), Via: ptr.To("10.0.0.1")},
			},
			expect: []network.RoutingData{{
				To:  netip.MustParsePrefix("0.0.0.0/0"),
				Via: netip.MustParseAddr("10.0.0.1"),
			}},
		},
		{
			name: "explicit is6 overrides via family",
			specs: []infrav1.RouteSpec{
				{To: ptr.To("default"), Is6: ptr.To(false), Via: ptr.To("10.0.0.1")},
			},
			expect: []network.RoutingData{{
				To:  netip.MustParsePrefix("0.0.0.0/0"),
				Via: netip.MustParseAddr("10.0.0.1"),
			}},
		},
		{
			name: "explicit prefix + table",
			specs: []infrav1.RouteSpec{
				{To: ptr.To("172.16.0.0/12"), Via: ptr.To("10.0.0.254"), Table: table100},
			},
			expect: []network.RoutingData{{
				To: netip.MustParsePrefix("172.16.0.0/12"), Table: table100,
				Via: netip.MustParseAddr("10.0.0.254"),
			}},
		},
		{
			name: "route without Via",
			specs: []infrav1.RouteSpec{
				{To: ptr.To("0.0.0.0/0")},
			},
			expect: []network.RoutingData{{
				To: netip.MustParsePrefix("0.0.0.0/0"),
			}},
		},
		{
			name: "multiple routes preserved in order",
			specs: []infrav1.RouteSpec{
				{To: ptr.To("0.0.0.0/0"), Via: ptr.To("10.0.0.1")},
				{To: ptr.To("::/0"), Via: ptr.To("2001:db8::1")},
			},
			expect: []network.RoutingData{
				{To: netip.MustParsePrefix("0.0.0.0/0"), Via: netip.MustParseAddr("10.0.0.1")},
				{To: netip.MustParsePrefix("::/0"), Via: netip.MustParseAddr("2001:db8::1")},
			},
		},
		{
			name:  "invalid To -> error",
			specs: []infrav1.RouteSpec{{To: ptr.To("not-an-ip")}},
			err:   true,
		},
		{
			name:  "prefix in Via -> error",
			specs: []infrav1.RouteSpec{{To: ptr.To("0.0.0.0/0"), Via: ptr.To("10.0.0.0/8")}},
			err:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToRoutingData(tc.specs)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expect, got)
		})
	}
}

func TestToFIBRuleData(t *testing.T) {
	table500 := ptr.To[int32](500)
	prio100 := ptr.To[int64](100)

	cases := []struct {
		name   string
		specs  []infrav1.RoutingPolicySpec
		expect []network.FIBRuleData
		err    bool
	}{
		{
			name:   "nil input mapped to empty slice",
			specs:  nil,
			expect: []network.FIBRuleData{},
		},
		{
			name: "to + table",
			specs: []infrav1.RoutingPolicySpec{
				{To: ptr.To("10.0.0.0/8"), Table: table500, Priority: prio100},
			},
			expect: []network.FIBRuleData{{
				To: netip.MustParsePrefix("10.0.0.0/8"), Table: table500,
				Priority: prio100,
			}},
		},
		{
			name: "from + table",
			specs: []infrav1.RoutingPolicySpec{
				{From: ptr.To("192.168.1.0/24"), Table: table500},
			},
			expect: []network.FIBRuleData{{
				Table: table500,
				From:  netip.MustParsePrefix("192.168.1.0/24"),
			}},
		},
		{
			name: "bare IP in From mapped to /32",
			specs: []infrav1.RoutingPolicySpec{
				{From: ptr.To("10.1.2.3"), Table: table500},
			},
			expect: []network.FIBRuleData{{
				Table: table500,
				From:  netip.PrefixFrom(netip.MustParseAddr("10.1.2.3"), 32),
			}},
		},
		{
			name: "to + from + priority + table",
			specs: []infrav1.RoutingPolicySpec{
				{To: ptr.To("0.0.0.0/0"), From: ptr.To("172.16.0.0/12"), Table: table500, Priority: prio100},
			},
			expect: []network.FIBRuleData{{
				To: netip.MustParsePrefix("0.0.0.0/0"), Table: table500,
				From:     netip.MustParsePrefix("172.16.0.0/12"),
				Priority: prio100,
			}},
		},
		{
			name: "Is6 disambiguates the From placeholder to ::/0",
			specs: []infrav1.RoutingPolicySpec{
				{From: ptr.To("all"), Is6: ptr.To(true), Table: table500},
			},
			expect: []network.FIBRuleData{{
				Table: table500,
				From:  netip.PrefixFrom(netip.IPv6Unspecified(), 0),
			}},
		},
		{
			name: "placeholder To family derived from concrete From without is6",
			specs: []infrav1.RoutingPolicySpec{
				{To: ptr.To("all"), From: ptr.To("2001:db8::/64"), Table: table500},
			},
			expect: []network.FIBRuleData{{
				To:    netip.PrefixFrom(netip.IPv6Unspecified(), 0),
				Table: table500,
				From:  netip.MustParsePrefix("2001:db8::/64"),
			}},
		},
		{
			name: "placeholder From family derived from concrete To without is6",
			specs: []infrav1.RoutingPolicySpec{
				{To: ptr.To("192.168.1.0/24"), From: ptr.To("default"), Table: table500},
			},
			expect: []network.FIBRuleData{{
				To:    netip.MustParsePrefix("192.168.1.0/24"),
				Table: table500,
				From:  netip.MustParsePrefix("0.0.0.0/0"),
			}},
		},
		{
			name:  "invalid To -> error",
			specs: []infrav1.RoutingPolicySpec{{To: ptr.To("no fun")}},
			err:   true,
		},
		{
			name:  "invalid From -> error",
			specs: []infrav1.RoutingPolicySpec{{From: ptr.To("allowed")}},
			err:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToFIBRuleData(tc.specs)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expect, got)
		})
	}
}
