/*
Copyright 2023-2024 IONOS Cloud.

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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1`

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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 196.168.100.124/24
      routes:
        - to: 0.0.0.0/0
          metric: 200
          via: 196.168.100.254
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
        - 10.10.10.12/24
        - '2001:db8::1/64'
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
        - to: '::/0'
          metric: 100
          via: '2001:db8::1'
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'`

	expectedValidNetworkConfigIPV6 = `network:
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
        - to: '::/0'
          metric: 100
          via: '2001:db8::1'
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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 10.10.11.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 200
          via: 10.10.11.1
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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 10.10.11.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 200
          via: 10.10.11.1
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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 196.168.100.124/24
      routes:
        - to: 0.0.0.0/0
          metric: 200
          via: 196.168.100.254
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
  vrfs:
    vrf-blue:
      table: 500
      routes:
        - { "to": "default",  "via": "192.168.178.1",  "metric": 100,  "table": 100, }
        - { "to": "10.10.10.0/24",  "via": "192.168.178.254",  "metric": 100, }
      routing-policy:
        - { "to": "0.0.0.0/0",  "from": "192.168.178.1/24",  "priority": 999,  "table": 100, }
      interfaces:
        - eth0
        - eth1`

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
        - 10.10.10.12/24
      routes:
        - to: 0.0.0.0/0
          metric: 100
          via: 10.10.10.1
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
        - 196.168.100.124/24
      routes:
        - to: 0.0.0.0/0
          metric: 200
          via: 196.168.100.254
      nameservers:
        addresses:
          - '8.8.8.8'
          - '8.8.4.4'
  vrfs:
    vrf-blue:
      table: 500
      routes:
        - { "to": "default",  "via": "192.168.178.1",  "metric": 100,  "table": 100, }
        - { "to": "10.10.10.0/24",  "via": "192.168.178.254",  "metric": 100, }
      routing-policy:
        - { "to": "0.0.0.0/0",  "from": "192.168.178.1/24",  "priority": 999,  "table": 100, }
      interfaces:
        - eth0
    vrf-red:
      table: 501
      routing-policy:
        - { "to": "0.0.0.0/0",  "from": "192.168.100.0/24",  "priority": 999,  "table": 101, }
      interfaces:
        - eth1`

	expectedValidNetworkConfigValidFIBRule = `network:
  version: 2
  renderer: networkd
  ethernets:
  vrfs:
    vrf-blue:
      table: 500
      routing-policy:
        - { "from": "10.10.0.0/16", }`
)

func TestNetworkConfig_Render(t *testing.T) {
	type args struct {
		nics []NetworkConfigData
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						LinkMTU:    ptr.To(uint16(9001)),
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP6:      true,
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP4:      true,
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP6:      true,
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					}, {
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "92:60:a0:5b:22:c3",
						IPAddress:  "10.10.11.12/24",
						Gateway:    "10.10.11.1",
						Metric:     ptr.To(uint32(200)),
						Routes: []RoutingData{{
							To:     "172.16.24.1/24",
							Metric: 50,
							Via:    "10.10.10.254",
						}, {
							To:  "2002::/64",
							Via: "2001:db8::1",
						},
						},
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						DHCP6:      true,
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					}, {
						Type:       "ethernet",
						Name:       "eth1",
						IPAddress:  "10.10.11.12/24",
						Gateway:    "10.10.11.1",
						Metric:     ptr.To(uint32(200)),
						MacAddress: "92:60:a0:5b:22:c3",
						FIBRules: []FIBRuleData{{
							To:       "0.0.0.0/0",
							From:     "192.168.178.1/24",
							Priority: 999,
							Table:    100,
						},
						},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigWithFIBRules,
				err:     nil,
			},
		},
		"InvalidNetworkConfigIp": {
			reason: "ip address is not set",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMissingIPAddress,
			},
		},
		"InvalidNetworkConfigMalformedIp": {
			reason: "malformed ip address",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.12",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMalformedIPAddress,
			},
		},
		"InvalidNetworkConfigMalformedIP": {
			reason: "ip address malformed",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.115",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMalformedIPAddress,
			},
		},
		"InvalidNetworkConfigGW": {
			reason: "gw is not set",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.12/24",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMissingGateway,
			},
		},
		"InvalidNetworkConfigMacAddress": {
			reason: "macaddress is not set",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						IPAddress:  "10.10.10.11/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMissingMacAddress,
			},
		},
		"InvalidNetworkConfigConflictingMetrics": {
			reason: "metric already exists for default gateway",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.11/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					}, {
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "92:60:a0:5b:22:c5",
						IPAddress:  "10.10.11.11/24",
						Gateway:    "10.10.11.254",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
					{
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "b4:87:18:bf:a3:60",
						IPAddress:  "196.168.100.124/24",
						Gateway:    "196.168.100.254",
						Metric:     ptr.To(uint32(200)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
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
				nics: []NetworkConfigData{},
			},
			want: want{
				network: "",
				err:     ErrMissingNetworkConfigData,
			},
		},
		"ValidNetworkConfigDualStack": {
			reason: "render valid network-config",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:        "ethernet",
						Name:        "eth0",
						MacAddress:  "92:60:a0:5b:22:c2",
						IPAddress:   "10.10.10.12/24",
						IPV6Address: "2001:db8::1/64",
						Gateway6:    "2001:db8::1",
						Metric6:     ptr.To(uint32(100)),
						Gateway:     "10.10.10.1",
						Metric:      ptr.To(uint32(100)),
						DNSServers:  []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigDualStack,
				err:     nil,
			},
		},
		"ValidNetworkConfigIPV6": {
			reason: "render valid ipv6 network-config",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:        "ethernet",
						Name:        "eth0",
						MacAddress:  "92:60:a0:5b:22:c2",
						IPV6Address: "2001:db8::1/64",
						Gateway6:    "2001:db8::1",
						Metric6:     ptr.To(uint32(100)),
						DNSServers:  []string{"8.8.8.8", "8.8.4.4"},
					},
				},
			},
			want: want{
				network: expectedValidNetworkConfigIPV6,
				err:     nil,
			},
		},
		"ValidNetworkConfigDHCP": {
			reason: "render valid network-config with dhcp",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
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
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
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
			reason: "valid config multiple nics enslaved to VRF",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
					{
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "b4:87:18:bf:a3:60",
						IPAddress:  "196.168.100.124/24",
						Gateway:    "196.168.100.254",
						Metric:     ptr.To(uint32(200)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
					{
						Type:       "vrf",
						Name:       "vrf-blue",
						Table:      500,
						Interfaces: []string{"eth0", "eth1"},
						Routes: []RoutingData{{
							To:     "default",
							Via:    "192.168.178.1",
							Metric: 100,
							Table:  100,
						}, {
							To:     "10.10.10.0/24",
							Via:    "192.168.178.254",
							Metric: 100,
						}},
						FIBRules: []FIBRuleData{{
							To:       "0.0.0.0/0",
							From:     "192.168.178.1/24",
							Priority: 999,
							Table:    100,
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
			reason: "valid config multiple nics enslaved to multiple VRFs",
			args: args{
				nics: []NetworkConfigData{
					{
						Type:       "ethernet",
						Name:       "eth0",
						MacAddress: "92:60:a0:5b:22:c2",
						IPAddress:  "10.10.10.12/24",
						Gateway:    "10.10.10.1",
						Metric:     ptr.To(uint32(100)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
					{
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "b4:87:18:bf:a3:60",
						IPAddress:  "196.168.100.124/24",
						Gateway:    "196.168.100.254",
						Metric:     ptr.To(uint32(200)),
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					},
					{
						Type:       "vrf",
						Name:       "vrf-blue",
						Table:      500,
						Interfaces: []string{"eth0"},
						Routes: []RoutingData{{
							To:     "default",
							Via:    "192.168.178.1",
							Metric: 100,
							Table:  100,
						}, {
							To:     "10.10.10.0/24",
							Via:    "192.168.178.254",
							Metric: 100,
						}},
						FIBRules: []FIBRuleData{{
							To:       "0.0.0.0/0",
							From:     "192.168.178.1/24",
							Priority: 999,
							Table:    100,
						}},
					},
					{
						Type:       "vrf",
						Name:       "vrf-red",
						Table:      501,
						Interfaces: []string{"eth1"},
						FIBRules: []FIBRuleData{{
							To:       "0.0.0.0/0",
							From:     "192.168.100.0/24",
							Priority: 999,
							Table:    101,
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
				nics: []NetworkConfigData{
					{
						Type:  "vrf",
						Name:  "vrf-blue",
						Table: 500,
						FIBRules: []FIBRuleData{{
							From: "10.10.0.0/16",
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
				nics: []NetworkConfigData{
					{
						Type:       "vrf",
						Name:       "vrf-blue",
						Table:      500,
						Interfaces: []string{"eth0", "eth1"},
						Routes: []RoutingData{{
							Table: 100,
						}},
					},
				},
			},
			want: want{
				network: "",
				err:     ErrMalformedRoute,
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
