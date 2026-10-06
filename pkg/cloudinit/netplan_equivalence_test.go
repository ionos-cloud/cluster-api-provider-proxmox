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
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The constants below are the exact network-config that the previous
// text/template renderer produced for two representative inputs (an ethernet
// with routes + DNS, and a pair of VRFs with routes, routing-policy and
// interfaces). Issue #780 replaced that renderer with structured YAML
// marshalling, which changes formatting (block vs inline flow maps, quoting
// style) but must not change meaning.
//
// TestNetplanRenderIsSemanticallyUnchanged parses these previous outputs and
// the current golden outputs into a canonical structure and asserts they are
// equal — i.e. the refactor changed only formatting, not the netplan config.
const (
	previousRenderWithRoutes = `network:
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

	previousRenderMultipleVRF = `network:
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
)

func TestNetplanRenderIsSemanticallyUnchanged(t *testing.T) {
	cases := []struct {
		name     string
		previous string
		current  string
	}{
		{"WithRoutes", previousRenderWithRoutes, expectedValidNetworkConfigWithRoutes},
		{"MultipleVRF", previousRenderMultipleVRF, expectedValidNetworkConfigMultipleNicsMultipleVRF},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var prev, cur any
			require.NoError(t, yaml.Unmarshal([]byte(tc.previous), &prev))
			require.NoError(t, yaml.Unmarshal([]byte(tc.current), &cur))
			require.Equal(t, prev, cur,
				"structured network-config changed meaning, not just formatting")
		})
	}
}
