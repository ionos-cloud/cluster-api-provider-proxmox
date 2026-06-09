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
	"maps"
	"net/netip"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

var (
	expectedValidNetworkConfig = map[string]string{
		"00-eth0.network": `[Match]
MACAddress=E2:B8:FE:E7:50:75
[Network]
DNS=10.0.1.1
[Address]
Address=10.0.0.98/25
[Address]
Address=2001:db8:1::10/64
[Route]
Destination=0.0.0.0/0
Gateway=10.0.0.1
Metric=100
[Route]
Destination=::/0
Gateway=2001:db8:1::1
Metric=100
`,
		"01-eth1.network": `[Match]
MACAddress=E2:8E:95:1F:EB:36
[Network]
DNS=10.0.1.1
[Address]
Address=10.0.1.84/25
[Route]
Destination=0.0.0.0/0
Gateway=10.0.1.1
Metric=200
[RoutingPolicyRule]
To=8.7.6.5/32
From=1.1.1.1/32
Priority=100
Table=500
`,
	}

	expectedValidNetworkdConfigEthernetWithRoutes = map[string]string{
		"00-eth0.network": `[Match]
MACAddress=E2:B8:FE:E7:50:75
[Network]
DNS=10.0.1.1
[Address]
Address=10.0.0.98/25
[Route]
Destination=0.0.0.0/0
Gateway=10.0.0.1
Metric=100
[Route]
Destination=172.16.24.0/24
Gateway=10.10.10.254
Metric=50
`,
	}

	expectedValidNetworkdConfigMultipleIPsSingleGateway = map[string]string{
		"00-eth0.network": `[Match]
MACAddress=E2:B8:FE:E7:50:75
[Network]
DNS=10.0.1.1
[Address]
Address=10.10.10.10/24
[Address]
Address=10.5.10.10/23
[Route]
Destination=0.0.0.0/0
Gateway=10.0.0.1
Metric=100
`,
	}

	expectedValidNetworkConfigWithVRFPolicies = map[string]string{
		"00-vrf0.netdev": `[NetDev]
Name=vrf0
Kind=vrf
[VRF]
Table=644
`,
		"00-eth0.network": `[Match]
MACAddress=E2:B8:FE:E7:50:75
[Network]
DNS=10.0.1.1
[Address]
Address=10.0.0.98/25
[Address]
Address=2001:db8:1::10/64
[Route]
Destination=0.0.0.0/0
Gateway=10.0.0.1
Metric=100
[Route]
Destination=::/0
Gateway=2001:db8:1::1
Metric=100
`,
		"01-eth1.network": `[Match]
MACAddress=E2:8E:95:1F:EB:36
[Network]
VRF=vrf0
DNS=10.0.1.1
[Address]
Address=10.0.1.84/25
[Route]
Destination=0.0.0.0/0
Gateway=10.0.1.1
Metric=200
`,

		"02-vrf2.network": `[Match]
Name=vrf0
[Route]
Destination=3.4.5.6/32
Gateway=10.0.1.1
Metric=100
Table=644
[RoutingPolicyRule]
To=8.7.6.5/32
From=1.1.1.1/32
Priority=100
Table=644
`,
	}
)

func TestRenderNetworkConfigData(t *testing.T) {
	type args struct {
		nics []network.NetworkConfigData
	}

	type want struct {
		units map[string]string
		err   error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValidNetworkdConfig": {
			reason: "render valid networkd with static ip",
			args: args{
				nics: []network.NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "E2:B8:FE:E7:50:75",
						IPConfigs: []network.IPConfig{
							{IPAddress: netip.MustParsePrefix("10.0.0.98/25")},
							{IPAddress: netip.MustParsePrefix("2001:db8:1::10/64")},
						},
						ProxName:   infrav1.DefaultNetworkDevice,
						DNSServers: []string{"10.0.1.1"},
						Routes: []network.RoutingData{{

							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.0.0.1"),
							Metric: ptr.To[int32](100),
						}, {

							To:     netip.MustParsePrefix("::/0"),
							Via:    netip.MustParseAddr("2001:db8:1::1"),
							Metric: ptr.To[int32](100),
						}},
					},
					{
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "E2:8E:95:1F:EB:36",
						IPConfigs: []network.IPConfig{
							{IPAddress: netip.MustParsePrefix("10.0.1.84/25")},
						},
						ProxName:   "net1",
						DNSServers: []string{"10.0.1.1"},
						FIBRules: []network.FIBRuleData{{

							To: netip.MustParsePrefix("8.7.6.5/32"), Table: ptr.To(int32(500)),
							From:     netip.MustParsePrefix("1.1.1.1/32"),
							Priority: ptr.To(int64(100)),
						}},
						Routes: []network.RoutingData{{

							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.0.1.1"),
							Metric: ptr.To[int32](200),
						}},
					},
				},
			},
			want: want{
				units: expectedValidNetworkConfig,
				err:   nil,
			},
		},
		"ValidNetworkdConfigEthernetWithRoutes": {
			reason: "render valid network config with ethernet and routes",
			args: args{
				nics: []network.NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "E2:B8:FE:E7:50:75",
						IPConfigs: []network.IPConfig{
							{IPAddress: netip.MustParsePrefix("10.0.0.98/25")},
						},
						ProxName:   infrav1.DefaultNetworkDevice,
						DNSServers: []string{"10.0.1.1"},
						Routes: []network.RoutingData{{

							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.0.0.1"),
							Metric: ptr.To(int32(100)),
						}, {

							To:     netip.MustParsePrefix("172.16.24.0/24"),
							Via:    netip.MustParseAddr("10.10.10.254"),
							Metric: ptr.To(int32(50)),
						}},
					},
				},
			},
			want: want{
				units: expectedValidNetworkdConfigEthernetWithRoutes,
				err:   nil,
			},
		},
		"ValidNetworkdConfigMultipleIPsSingleGateway": {
			reason: "renter valid network config with multiple gateways and single gateway",
			args: args{
				nics: []network.NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "E2:B8:FE:E7:50:75",
						IPConfigs: []network.IPConfig{
							{IPAddress: netip.MustParsePrefix("10.10.10.10/24")},
							{IPAddress: netip.MustParsePrefix("10.5.10.10/23")},
						},
						ProxName:   infrav1.DefaultNetworkDevice,
						DNSServers: []string{"10.0.1.1"},
						Routes: []network.RoutingData{{

							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.0.0.1"),
							Metric: ptr.To(int32(100)),
						}},
					},
				},
			},
			want: want{
				units: expectedValidNetworkdConfigMultipleIPsSingleGateway,
				err:   nil,
			},
		},
		"ValidNetworkdConfigWithVRFPolicies": {
			reason: "render valid networkd with static ip and VRF and policies",
			args: args{
				nics: []network.NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "E2:B8:FE:E7:50:75",
						IPConfigs: []network.IPConfig{
							{IPAddress: netip.MustParsePrefix("10.0.0.98/25")},
							{IPAddress: netip.MustParsePrefix("2001:db8:1::10/64")},
						},
						ProxName:   infrav1.DefaultNetworkDevice,
						DNSServers: []string{"10.0.1.1"},
						Routes: []network.RoutingData{{

							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.0.0.1"),
							Metric: ptr.To[int32](100),
						}, {

							To:     netip.MustParsePrefix("::/0"),
							Via:    netip.MustParseAddr("2001:db8:1::1"),
							Metric: ptr.To[int32](100),
						}},
					},
					{
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "E2:8E:95:1F:EB:36",
						IPConfigs: []network.IPConfig{
							{IPAddress: netip.MustParsePrefix("10.0.1.84/25")},
						},
						ProxName:   "net1",
						DNSServers: []string{"10.0.1.1"},
						Routes: []network.RoutingData{{

							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.0.1.1"),
							Metric: ptr.To[int32](200),
						}},
					},
					{
						Type:     "vrf",
						Name:     "vrf0",
						ProxName: "net1",
						Table:    ptr.To(int32(644)),
						Children: []string{"eth1"},
						Routes: []network.RoutingData{{

							To:     netip.PrefixFrom(netip.MustParseAddr("3.4.5.6"), 32),
							Via:    netip.MustParseAddr("10.0.1.1"),
							Metric: ptr.To(int32(100)),
						}},
						FIBRules: []network.FIBRuleData{{

							To:       netip.MustParsePrefix("8.7.6.5/32"),
							From:     netip.MustParsePrefix("1.1.1.1/32"),
							Priority: ptr.To(int64(100)),
						}},
					},
				},
			},
			want: want{
				units: expectedValidNetworkConfigWithVRFPolicies,
				err:   nil,
			},
		},
	}

	for n, tc := range cases {
		t.Run(n, func(t *testing.T) {
			units, err := RenderNetworkConfigData(tc.args.nics)
			require.ErrorIs(t, err, tc.want.err)
			require.ElementsMatch(t, slices.Collect(maps.Keys(tc.want.units)), slices.Collect(maps.Keys(units)))
			for k, want := range tc.want.units {
				require.Equal(t, want, string(units[k]))
			}
		})
	}
}
