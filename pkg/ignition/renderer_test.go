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
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

// validEthernet returns a single, renderable ethernet device with a default
// route so that the shared gateway check in network.Network passes.
func validEthernet() network.ConfigData {
	return network.ConfigData{
		Type:       network.TypeEthernet,
		Name:       "eth0",
		MacAddress: "E2:B8:FE:E7:50:75",
		IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.0.0.98/25")}},
		DNSServers: []string{"10.0.1.1"},
		Routes: []network.RoutingData{{
			To:     netip.MustParsePrefix("0.0.0.0/0"),
			Via:    netip.MustParseAddr("10.0.0.1"),
			Metric: ptr.To[int32](100),
		}},
	}
}

func TestNetworkConfig_Validate(t *testing.T) {
	cases := map[string]struct {
		reason string
		nics   []network.ConfigData
		err    error
	}{
		"Valid": {
			reason: "a complete ethernet device with a default gateway is valid",
			nics:   []network.ConfigData{validEthernet()},
			err:    nil,
		},
		"NoDevices": {
			reason: "an empty device list fails the shared structural check",
			nics:   nil,
			err:    ErrMissingNetworkConfigData,
		},
		"NoGateway": {
			reason: "a device set without a default gateway fails the shared check",
			nics: func() []network.ConfigData {
				d := validEthernet()
				d.Routes = nil
				return []network.ConfigData{d}
			}(),
			err: ErrMissingGateway,
		},
	}

	for n, tc := range cases {
		t.Run(n, func(t *testing.T) {
			err := NewNetworkConfig(tc.nics).Validate()
			require.ErrorIs(t, err, tc.err, tc.reason)
		})
	}
}

func TestNetworkConfig_Render(t *testing.T) {
	t.Run("RendersUnitFiles", func(t *testing.T) {
		units, err := NewNetworkConfig([]network.ConfigData{validEthernet()}).Render()
		require.NoError(t, err)

		// Render delegates to RenderNetworkConfigData; assert the produced unit
		// matches that contract.
		want, err := RenderNetworkConfigData([]network.ConfigData{validEthernet()})
		require.NoError(t, err)
		require.Equal(t, want, units)
	})

	t.Run("ValidationGatesRender", func(t *testing.T) {
		// An invalid config must not be rendered.
		units, err := NewNetworkConfig(nil).Render()
		require.ErrorIs(t, err, ErrMissingNetworkConfigData)
		require.Nil(t, units)
	})
}

func TestNetworkConfig_Inspect(t *testing.T) {
	nics := []network.ConfigData{validEthernet()}
	got, err := NewNetworkConfig(nics).Inspect()
	require.NoError(t, err)

	var roundTrip []network.ConfigData
	require.NoError(t, json.Unmarshal(got, &roundTrip))
	require.Equal(t, nics, roundTrip)
}
