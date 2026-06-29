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

// NOSONAR
package cloudinit

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

const (
	expectedValidNetworkConfig = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigWithLinkMTU = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
      mtu: 9001`

	expectedValidNetworkConfigWithoutDNS = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }`

	expectedValidNetworkConfigMultipleNics = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
    eth1:
      match:
        macaddress: b4:87:18:bf:a3:60
      dhcp4: false
      dhcp6: false
      addresses:
        - '192.168.100.124/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "192.168.100.254",  "metric": 200, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigDualStack = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
        - '2001:db8::1/64'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
        - { "to": "::/0",  "via": "2001:db8::1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigMultipleNetsOneGateway = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
        - '2001:db8::1/64'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
    eth1:
      match:
        macaddress: 92:60:a0:5b:22:c3
      dhcp4: false
      dhcp6: false
      addresses:
        - '2001:db8::1/64'
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigIPv6 = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '2001:db8::1/64'
      routes:
        - { "to": "::/0",  "via": "2001:db8::1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigDHCP = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: true
      dhcp6: true
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigDHCP4 = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: true
      dhcp6: false
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigDHCP6 = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: true
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigWithDHCP = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: true
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigIPAndDHCP = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: true
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigWithRoutes = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: true
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
    eth1:
      match:
        macaddress: 92:60:a0:5b:22:c3
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.11.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.11.1",  "metric": 200, }
        - { "to": "172.16.24.1/24",  "via": "10.10.10.254",  "metric": 50, }
        - { "to": "2002::/64",  "via": "2001:db8::1", }`

	expectedValidNetworkConfigWithFIBRules = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: true
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
    eth1:
      match:
        macaddress: 92:60:a0:5b:22:c3
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.11.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.11.1",  "metric": 200, }
      routing-policy:
        - { "to": "0.0.0.0/0",  "from": "192.168.178.1/24",  "priority": 999,  "table": 100, }`

	expectedValidNetworkConfigMultipleNicsVRF = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
    eth1:
      match:
        macaddress: b4:87:18:bf:a3:60
      dhcp4: false
      dhcp6: false
      addresses:
        - '192.168.100.124/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "192.168.100.254",  "metric": 200, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
  vrfs:
    vrf-blue:
      table: 500
      routes:
        - { "to": "0.0.0.0/0",  "via": "192.168.178.1",  "metric": 100,  "table": 100, }
        - { "to": "10.10.10.0/24",  "via": "192.168.178.254",  "metric": 100, }
      routing-policy:
        - { "to": "0.0.0.0/0",  "from": "192.168.178.1/24",  "priority": 999,  "table": 100, }
      interfaces:
        - 'eth0'
        - 'eth1'`

	expectedValidNetworkConfigMultipleNicsMultipleVRF = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
    eth1:
      match:
        macaddress: b4:87:18:bf:a3:60
      dhcp4: false
      dhcp6: false
      addresses:
        - '192.168.100.124/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "192.168.100.254",  "metric": 200, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
  vrfs:
    vrf-blue:
      table: 500
      routes:
        - { "to": "0.0.0.0/0",  "via": "192.168.178.1",  "metric": 100,  "table": 100, }
        - { "to": "10.10.10.0/24",  "via": "192.168.178.254",  "metric": 100, }
      routing-policy:
        - { "to": "0.0.0.0/0",  "from": "192.168.178.1/24",  "priority": 999,  "table": 100, }
      interfaces:
        - 'eth0'
    vrf-red:
      table: 501
      routing-policy:
        - { "to": "0.0.0.0/0",  "from": "192.168.100.0/24",  "priority": 999,  "table": 101, }
      interfaces:
        - 'eth1'`

	expectedYamlEdgeCases = `network:
  version: 2
  renderer: networkd
  ethernets:
    NO &anchor:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: false
      dhcp6: false
      addresses:
        - '10.10.10.12/24'
      routes:
        - { "to": "0.0.0.0/0",  "via": "10.10.10.1",  "metric": 100, }
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
    asdf !.tag:
      match:
        macaddress: b4:87:18:bf:a3:60
      dhcp4: false
      dhcp6: false
      addresses:
        - '2001:db8::ffff:0/64'
      routes:
        - { "to": "::/0",  "via": "2001:db8::",  "metric": 200, }
      nameservers:
        addresses:
          - '22:22'
          - '::'
          - '[::]'
  vrfs:
    vrf-blue:
      table: 500
      routes:
        - { "to": "::/128",  "via": "192.168.178.1", }
      interfaces:
        - 'NO &anchor'
        - 'asdf !.tag'`

	expectedValidNetworkConfigValidFIBRule = `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      match:
        macaddress: 92:60:a0:5b:22:c2
      dhcp4: true
      dhcp6: false
  vrfs:
    vrf-blue:
      table: 500
      routing-policy:
        - { "from": "10.10.0.0/16", }`
)

func TestNetworkConfig_Render(t *testing.T) {
	type args struct {
		nics []network.ConfigData
	}

	type want struct {
		network string
		err     error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValidStaticNetworkConfig": {
			reason: "render valid network-config with static ip",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfig,
				err:     nil,
			},
		},
		"ValidStaticNetworkConfigWithLinkMTU": {
			reason: "render valid network-config with static ip and mtu",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						LinkMTU:    new(int32(9001)),
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigWithLinkMTU,
				err:     nil,
			},
		},
		"ValidStaticNetworkConfigWithDHCP": {
			reason: "render valid network-config with ipv6 static ip and dhcp",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP6:      true,
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigWithDHCP,
				err:     nil,
			},
		},
		"ValidStaticNetworkConfigIPWithDHCP": {
			reason: "render valid network-config with ipv6 static ip and dhcp",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true,
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigIPAndDHCP,
				err:     nil,
			},
		},
		"ValidStaticNetworkConfigWithRoutes": {
			reason: "render valid network-config with ipv6 static ip and dhcp and routes",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP6:      true,
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					}, {
						Type:       network.TypeEthernet,
						Name:       "eth1",
						MacAddress: "92:60:a0:5b:22:c3",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.11.12/24"),
						}},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.11.1"),
							Metric: new(int32(200)),
						}, {

							To:     netip.MustParsePrefix("172.16.24.1/24"),
							Metric: new(int32(50)),
							Via:    netip.MustParseAddr("10.10.10.254"),
						}, {

							To:  netip.MustParsePrefix("2002::/64"),
							Via: netip.MustParseAddr("2001:db8::1"),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigWithRoutes,
				err:     nil,
			},
		},
		"ValidStaticNetworkConfigWithFIBRules": {
			reason: "render valid network-config with FIB rules/routing policy",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP6:      true,
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					}, {
						Type: network.TypeEthernet,
						Name: "eth1",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.11.12/24"),
						}},
						MacAddress: "92:60:a0:5b:22:c3",
						FIBRules: []network.FIBRuleData{{
							To: netip.MustParsePrefix("0.0.0.0/0"), Table: new(int32(100)),
							From:     netip.MustParsePrefix("192.168.178.1/24"),
							Priority: new(int64(999)),
						}},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.11.1"),
							Metric: new(int32(200)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigWithFIBRules,
				err:     nil,
			},
		},
		"InvalidNetworkConfigGW": {
			reason: "gw is not set",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMissingGateway,
			},
		},
		"InvalidNetworkConfigConflictingMetrics": {
			reason: "metric already exists for default gateway multiple network cards",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.11/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					}, {
						Type:       network.TypeEthernet,
						Name:       "eth1",
						MacAddress: "92:60:a0:5b:22:c5",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.11.11/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.11.1"),
							Metric: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrConflictingMetrics,
			},
		},
		"ValidNetworkConfigWithoutDNS": {
			reason: "valid config without dns",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigWithoutDNS,
				err:     nil,
			},
		},
		"ValidNetworkConfigMultipleNics": {
			reason: "valid config multiple nics",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
					{
						Type:       network.TypeEthernet,
						Name:       "eth1",
						MacAddress: "b4:87:18:bf:a3:60",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("192.168.100.124/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("192.168.100.254"),
							Metric: new(int32(200)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigMultipleNics,
				err:     nil,
			},
		},
		"InvalidNetworkConfigData": {
			reason: "invalid config missing network config data",
			args: args{
				nics: []network.ConfigData{},
			},
			want: want{
				network: "",
				err:     ErrMissingNetworkConfigData,
			},
		},
		"ValidNetworkConfigDualStack": {
			reason: "render valid network-config",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}, {
							IPAddress: netip.MustParsePrefix("2001:db8::1/64"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}, {

							To:     netip.MustParsePrefix("::/0"),
							Via:    netip.MustParseAddr("2001:db8::1"),
							Metric: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigDualStack,
				err:     nil,
			},
		},
		"ValidNetworkConfigMultipleNetsOneGateway": {
			reason: "render valid network-config with one gateway",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}, {
							IPAddress: netip.MustParsePrefix("2001:db8::1/64"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
					{
						Type:       network.TypeEthernet,
						Name:       "eth1",
						MacAddress: "92:60:a0:5b:22:c3",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("2001:db8::1/64"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigMultipleNetsOneGateway,
				err:     nil,
			},
		},
		"ValidNetworkConfigIPv6": {
			reason: "render valid ipv6 network-config",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("2001:db8::1/64"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("::/0"),
							Via:    netip.MustParseAddr("2001:db8::1"),
							Metric: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigIPv6,
				err:     nil,
			},
		},
		"ValidNetworkConfigDHCP": {
			reason: "render valid network-config with dhcp",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true,
						DHCP6:      true,
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigDHCP,
				err:     nil,
			},
		},
		"ValidNetworkConfigDHCP4": {
			reason: "render valid network-config with dhcp",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true,
						DHCP6:      false,
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigDHCP4,
				err:     nil,
			},
		},
		"ValidNetworkConfigDHCP6": {
			reason: "render valid network-config with dhcp",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      false,
						DHCP6:      true,
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigDHCP6,
				err:     nil,
			},
		},
		"ValidNetworkConfigMultipleNicsVRF": {
			reason: "valid config multiple nics attached to VRF",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
					{
						Type:       network.TypeEthernet,
						Name:       "eth1",
						MacAddress: "b4:87:18:bf:a3:60",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("192.168.100.124/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("192.168.100.254"),
							Metric: new(int32(200)),
						}},
					},
					{
						Type:     network.TypeVRF,
						Name:     "vrf-blue",
						Table:    new(int32(500)),
						Children: []string{"eth0", "eth1"},
						Routes: []network.RoutingData{{
							To: netip.MustParsePrefix("0.0.0.0/0"), Table: new(int32(100)),
							Via:    netip.MustParseAddr("192.168.178.1"),
							Metric: new(int32(100)),
						}, {

							To:     netip.MustParsePrefix("10.10.10.0/24"),
							Via:    netip.MustParseAddr("192.168.178.254"),
							Metric: new(int32(100)),
						}},
						FIBRules: []network.FIBRuleData{{
							To: netip.MustParsePrefix("0.0.0.0/0"), Table: new(int32(100)),
							From:     netip.MustParsePrefix("192.168.178.1/24"),
							Priority: new(int64(999)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigMultipleNicsVRF,
				err:     nil,
			},
		},
		"ValidNetworkConfigMultipleNicsMultipleVRF": {
			reason: "valid config multiple nics attached to multiple VRFs",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
					{
						Type:       network.TypeEthernet,
						Name:       "eth1",
						MacAddress: "b4:87:18:bf:a3:60",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("192.168.100.124/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("192.168.100.254"),
							Metric: new(int32(200)),
						}},
					},
					{
						Type:     network.TypeVRF,
						Name:     "vrf-blue",
						Table:    new(int32(500)),
						Children: []string{"eth0"},
						Routes: []network.RoutingData{{
							To: netip.MustParsePrefix("0.0.0.0/0"), Table: new(int32(100)),
							Via:    netip.MustParseAddr("192.168.178.1"),
							Metric: new(int32(100)),
						}, {

							To:     netip.MustParsePrefix("10.10.10.0/24"),
							Via:    netip.MustParseAddr("192.168.178.254"),
							Metric: new(int32(100)),
						}},
						FIBRules: []network.FIBRuleData{{
							To: netip.MustParsePrefix("0.0.0.0/0"), Table: new(int32(100)),
							From:     netip.MustParsePrefix("192.168.178.1/24"),
							Priority: new(int64(999)),
						}},
					},
					{
						Type:     network.TypeVRF,
						Name:     "vrf-red",
						Table:    new(int32(501)),
						Children: []string{"eth1"},
						FIBRules: []network.FIBRuleData{{
							To: netip.MustParsePrefix("0.0.0.0/0"), Table: new(int32(101)),
							From:     netip.MustParsePrefix("192.168.100.0/24"),
							Priority: new(int64(999)),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigMultipleNicsMultipleVRF,
				err:     nil,
			},
		},
		"ValidNetworkConfigValidFIBRule": {
			reason: "valid config valid routing policy",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true, // satisfy gateway requirement.
					},
					{
						Type:  network.TypeVRF,
						Name:  "vrf-blue",
						Table: new(int32(500)),
						FIBRules: []network.FIBRuleData{{
							From: netip.MustParsePrefix("10.10.0.0/16"),
						}},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigValidFIBRule,
				err:     nil,
			},
		},
		"InvalidNetworkConfigMalformedFIBRule": {
			reason: "invalid config malformed routing policy",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true, // satisfy gateway requirement.
					},
					{
						Type:       network.TypeEthernet,
						Name:       "eth1",
						MacAddress: "92:60:a0:5b:22:c3",
						DHCP4:      true,
					},
					{
						Type:     network.TypeVRF,
						Name:     "vrf-blue",
						Table:    new(int32(500)),
						Children: []string{"eth0", "eth1"},
						Routes: []network.RoutingData{{
							Table: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMalformedRoute,
			},
		},
		"InvalidNetworkConfigMalformedRouteOnEthernet": {
			reason: "invalid config malformed route for ethernet",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true,
						Routes: []network.RoutingData{{
							Table: new(int32(100)),
						}},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMalformedRoute,
			},
		},
		"InvalidNetworkConfigDuplicateGateway": {
			reason: "invalid config multiple routes",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true,
						Routes: []network.RoutingData{{
							Table: new(int32(100)), To: netip.MustParsePrefix("0.0.0.0/0"),
						}, {

							Table: new(int32(100)), To: netip.MustParsePrefix("0.0.0.0/0"),
						}},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrConflictingMetrics,
			},
		},
		"InvalidNetworkConfigFIBRuleMissingTableOnEthernet": {
			reason: "invalid config missing table for FIB rule on ethernet",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true,
						FIBRules: []network.FIBRuleData{{
							From: netip.MustParsePrefix("10.10.0.0/16"),
						}},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMalformedFIBRule,
			},
		},
		"InvalidNetworkConfigFIBRuleMissingFromAndToOnEthernet": {
			reason: "invalid config FIB rule for ethernet requires match",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true,
						FIBRules: []network.FIBRuleData{{
							Table:    new(int32(100)),
							Priority: new(int64(100)),
						}},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMalformedFIBRule,
			},
		},
		"YamlEdgeCases": {
			reason: "valid config multiple nics attached to multiple VRFs",
			args: args{
				nics: []network.ConfigData{
					{
						Type:       network.TypeEthernet,
						Name:       "NO &anchor",
						MacAddress: "92:60:a0:5b:22:c2",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("10.10.10.12/24"),
						}},
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("0.0.0.0/0"),
							Via:    netip.MustParseAddr("10.10.10.1"),
							Metric: new(int32(100)),
						}},
					},
					{
						Type:       network.TypeEthernet,
						Name:       "asdf !.tag",
						MacAddress: "b4:87:18:bf:a3:60",
						IPConfigs: []network.IPConfig{{
							IPAddress: netip.MustParsePrefix("2001:db8::ffff:0/64"),
						}},
						DNSServers: []string{"22:22", "::", "[::]"},
						Routes: []network.RoutingData{{
							To:     netip.MustParsePrefix("::/0"),
							Via:    netip.MustParseAddr("2001:db8::"),
							Metric: new(int32(200)),
						}},
					},
					{
						Type:     network.TypeVRF,
						Name:     "vrf-blue",
						Table:    new(int32(500)),
						Children: []string{"NO &anchor", "asdf !.tag"},
						Routes: []network.RoutingData{{
							To:  netip.PrefixFrom(netip.MustParseAddr("::"), 128),
							Via: netip.MustParseAddr("192.168.178.1"),
						}},
					},
				}},
			want: want{
				network: expectedYamlEdgeCases,
				err:     nil,
			},
		},
	}

	for n, tc := range cases {
		t.Run(n, func(t *testing.T) {
			nc := NewNetworkConfig(tc.args.nics)
			network, err := nc.Render()
			require.ErrorIs(t, err, tc.want.err)
			require.Equal(t, tc.want.network, string(network))
		})
	}
}
