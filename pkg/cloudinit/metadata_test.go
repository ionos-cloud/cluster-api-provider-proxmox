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

package cloudinit

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

const (
	expectedValidMetadata = `instance-id: 9a82e2ca-4294-11ee-be56-0242ac120002
local-hostname: proxmox-control-plane
hostname: proxmox-control-plane
provider-id: proxmox://9a82e2ca-4294-11ee-be56-0242ac120002
kubernetes-version: 1.2.3
`
	expectedValidMetadataWithoutProviderID = `instance-id: 9a82e2ca-4294-11ee-be56-0242ac120002
local-hostname: proxmox-control-plane
hostname: proxmox-control-plane
kubernetes-version: 1.2.3
`
	expectedValidMetadataWithoutKubernetesVersion = `instance-id: 9a82e2ca-4294-11ee-be56-0242ac120002
local-hostname: proxmox-control-plane
hostname: proxmox-control-plane
provider-id: proxmox://9a82e2ca-4294-11ee-be56-0242ac120002
`
)

func TestMetadata_Render(t *testing.T) {
	type args struct {
		instanceID        string
		hostname          string
		kubernetesVersion string
		injectProviderID  bool
	}

	type want struct {
		metadata string
		err      error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValidCloudinit": {
			reason: "rendering metadata",
			args: args{
				instanceID:        "9a82e2ca-4294-11ee-be56-0242ac120002",
				hostname:          "proxmox-control-plane",
				kubernetesVersion: "1.2.3",
				injectProviderID:  true,
			},
			want: want{
				metadata: expectedValidMetadata,
				err:      nil,
			},
		},
		"InvalidCloudinitMissingInstanceID": {
			reason: "instance-id is not set",
			args: args{
				hostname: "some-hostname",
			},
			want: want{
				metadata: "",
				err:      ErrMissingInstanceID,
			},
		},
		"InvalidCloudinitMissingHostname": {
			reason: "hostname is not set",
			args:   args{},
			want: want{
				metadata: "",
				err:      ErrMissingHostname,
			},
		},
		"ValidCloudinitwithoutProviderID": {
			reason: "rendering metadata if providerID is not injected",
			args: args{
				instanceID:        "9a82e2ca-4294-11ee-be56-0242ac120002",
				hostname:          "proxmox-control-plane",
				kubernetesVersion: "1.2.3",
				injectProviderID:  false,
			},
			want: want{
				metadata: expectedValidMetadataWithoutProviderID,
				err:      nil,
			},
		},
		"ValidCloudinitwithoutKubernetesVersion": {
			reason: "rendering metadata if kubernetesVersion is not provided",
			args: args{
				instanceID:        "9a82e2ca-4294-11ee-be56-0242ac120002",
				hostname:          "proxmox-control-plane",
				kubernetesVersion: "",
				injectProviderID:  true,
			},
			want: want{
				metadata: expectedValidMetadataWithoutKubernetesVersion,
				err:      nil,
			},
		},
	}

	for n, tc := range cases {
		t.Run(n, func(t *testing.T) {
			ci := NewMetadata(MetadataInput{
				InstanceID:          tc.args.instanceID,
				Hostname:            tc.args.hostname,
				KubernetesVersion:   tc.args.kubernetesVersion,
				ProviderIDInjection: tc.args.injectProviderID,
			})
			metadata, err := ci.Render()
			require.ErrorIs(t, err, tc.want.err)
			require.Equal(t, tc.want.metadata, string(metadata))
		})
	}
}

const baseMetadataHeader = `instance-id: 9a82e2ca-4294-11ee-be56-0242ac120002
local-hostname: proxmox-control-plane
hostname: proxmox-control-plane
`

const (
	testIPv4    = "10.10.0.5"
	testIPv4GW  = "10.10.0.1"
	testIPv6    = "2001:db8::5"
	testIPv6GW  = "2001:db8::1"
	testNet0    = "net0"
	testNet1    = "net1"
	testIPv4Net = "10.20.0.5"
)

func baseInput() MetadataInput {
	return MetadataInput{
		InstanceID: "9a82e2ca-4294-11ee-be56-0242ac120002",
		Hostname:   "proxmox-control-plane",
	}
}

func TestMetadata_Render_WithIPAddresses(t *testing.T) {
	cases := map[string]struct {
		input    MetadataInput
		expected string
	}{
		"NoIPs": {
			input:    baseInput(),
			expected: baseMetadataHeader,
		},
		"IPv4OnlyDefault": {
			input: func() MetadataInput {
				in := baseInput()
				in.IPv4 = testIPv4
				in.IPv4Prefix = "24"
				in.IPv4Gateway = testIPv4GW
				in.NetworkAddresses = []NetworkAddress{{
					DeviceName: testNet0, IPv4: testIPv4, IPv4Prefix: "24", IPv4Gateway: testIPv4GW,
				}}
				return in
			}(),
			expected: baseMetadataHeader + `ipv4: 10.10.0.5
ipv4_prefix: 24
ipv4_gateway: 10.10.0.1
ipv4_net0: 10.10.0.5
ipv4_prefix_net0: 24
ipv4_gateway_net0: 10.10.0.1
`,
		},
		"IPv6OnlyDefault": {
			input: func() MetadataInput {
				in := baseInput()
				in.IPv6 = testIPv6
				in.IPv6Prefix = "64"
				in.IPv6Gateway = testIPv6GW
				in.NetworkAddresses = []NetworkAddress{{
					DeviceName: testNet0, IPv6: testIPv6, IPv6Prefix: "64", IPv6Gateway: testIPv6GW,
				}}
				return in
			}(),
			expected: baseMetadataHeader + `ipv6: 2001:db8::5
ipv6_prefix: 64
ipv6_gateway: 2001:db8::1
ipv6_net0: 2001:db8::5
ipv6_prefix_net0: 64
ipv6_gateway_net0: 2001:db8::1
`,
		},
		"DualStackDefault": {
			input: func() MetadataInput {
				in := baseInput()
				in.IPv4 = testIPv4
				in.IPv4Prefix = "24"
				in.IPv4Gateway = testIPv4GW
				in.IPv6 = testIPv6
				in.IPv6Prefix = "64"
				in.IPv6Gateway = testIPv6GW
				in.NetworkAddresses = []NetworkAddress{{
					DeviceName:  testNet0,
					IPv4:        testIPv4,
					IPv4Prefix:  "24",
					IPv4Gateway: testIPv4GW,
					IPv6:        testIPv6,
					IPv6Prefix:  "64",
					IPv6Gateway: testIPv6GW,
				}}
				return in
			}(),
			expected: baseMetadataHeader + `ipv4: 10.10.0.5
ipv4_prefix: 24
ipv4_gateway: 10.10.0.1
ipv6: 2001:db8::5
ipv6_prefix: 64
ipv6_gateway: 2001:db8::1
ipv4_net0: 10.10.0.5
ipv4_prefix_net0: 24
ipv4_gateway_net0: 10.10.0.1
ipv6_net0: 2001:db8::5
ipv6_prefix_net0: 64
ipv6_gateway_net0: 2001:db8::1
`,
		},
		"MultiNICDualStack": {
			input: func() MetadataInput {
				in := baseInput()
				in.IPv4 = testIPv4
				in.IPv4Prefix = "24"
				in.IPv4Gateway = testIPv4GW
				in.IPv6 = testIPv6
				in.IPv6Prefix = "64"
				in.IPv6Gateway = testIPv6GW
				in.NetworkAddresses = []NetworkAddress{
					{
						DeviceName:  testNet0,
						IPv4:        testIPv4,
						IPv4Prefix:  "24",
						IPv4Gateway: testIPv4GW,
						IPv6:        testIPv6,
						IPv6Prefix:  "64",
						IPv6Gateway: testIPv6GW,
					},
					{
						DeviceName: testNet1,
						IPv4:       testIPv4Net,
						IPv4Prefix: "24",
					},
				}
				return in
			}(),
			expected: baseMetadataHeader + `ipv4: 10.10.0.5
ipv4_prefix: 24
ipv4_gateway: 10.10.0.1
ipv6: 2001:db8::5
ipv6_prefix: 64
ipv6_gateway: 2001:db8::1
ipv4_net0: 10.10.0.5
ipv4_prefix_net0: 24
ipv4_gateway_net0: 10.10.0.1
ipv6_net0: 2001:db8::5
ipv6_prefix_net0: 64
ipv6_gateway_net0: 2001:db8::1
ipv4_net1: 10.20.0.5
ipv4_prefix_net1: 24
`,
		},
		"PartialMissingGateway": {
			input: func() MetadataInput {
				in := baseInput()
				in.IPv4 = testIPv4
				in.IPv4Prefix = "24"
				// Gateway intentionally empty.
				in.NetworkAddresses = []NetworkAddress{{
					DeviceName: testNet0, IPv4: testIPv4, IPv4Prefix: "24",
				}}
				return in
			}(),
			expected: baseMetadataHeader + `ipv4: 10.10.0.5
ipv4_prefix: 24
ipv4_net0: 10.10.0.5
ipv4_prefix_net0: 24
`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ci := NewMetadata(tc.input)
			out, err := ci.Render()
			require.NoError(t, err)
			require.Equal(t, tc.expected, string(out))
		})
	}
}

func TestMetadataInput_PopulateNetworkAddresses(t *testing.T) {
	t.Run("EmptyNICDataLeavesInputUnchanged", func(t *testing.T) {
		in := baseInput()
		in.PopulateNetworkAddresses(nil)
		require.Empty(t, in.IPv4)
		require.Empty(t, in.IPv6)
		require.Empty(t, in.NetworkAddresses)
	})

	t.Run("DualStackDefaultIPsArePromotedToAliases", func(t *testing.T) {
		nicData := []network.ConfigData{
			{
				ProxName: infrav1.DefaultNetworkDevice,
				IPConfigs: []network.IPConfig{
					{IPAddress: netip.MustParsePrefix("10.10.0.5/24"), Default: true},
					{IPAddress: netip.MustParsePrefix("2001:db8::5/64"), Default: true},
				},
				Routes: []network.RoutingData{
					{To: netip.MustParsePrefix("0.0.0.0/0"), Via: netip.MustParseAddr("10.10.0.1")},
					{To: netip.MustParsePrefix("::/0"), Via: netip.MustParseAddr("2001:db8::1")},
				},
			},
		}
		in := baseInput()
		in.PopulateNetworkAddresses(nicData)
		require.Equal(t, "10.10.0.5", in.IPv4)
		require.Equal(t, "24", in.IPv4Prefix)
		require.Equal(t, "10.10.0.1", in.IPv4Gateway)
		require.Equal(t, "2001:db8::5", in.IPv6)
		require.Equal(t, "64", in.IPv6Prefix)
		require.Equal(t, "2001:db8::1", in.IPv6Gateway)
		require.Len(t, in.NetworkAddresses, 1)
		require.Equal(t, "net0", in.NetworkAddresses[0].DeviceName)
		require.Equal(t, "10.10.0.5", in.NetworkAddresses[0].IPv4)
		require.Equal(t, "2001:db8::5", in.NetworkAddresses[0].IPv6)
	})

	t.Run("PerDeviceEntryUsedEvenWhenNotDefault", func(t *testing.T) {
		nicData := []network.ConfigData{
			{
				ProxName: "net1",
				IPConfigs: []network.IPConfig{
					{IPAddress: netip.MustParsePrefix("10.20.0.5/24"), Default: false},
				},
			},
		}
		in := baseInput()
		in.PopulateNetworkAddresses(nicData)
		require.Empty(t, in.IPv4, "no default IP should populate the alias")
		require.Len(t, in.NetworkAddresses, 1)
		require.Equal(t, "net1", in.NetworkAddresses[0].DeviceName)
		require.Equal(t, "10.20.0.5", in.NetworkAddresses[0].IPv4)
	})

	t.Run("DefaultIPWinsOverNonDefaultOnSameNIC", func(t *testing.T) {
		// Two IPv4 addresses on the same NIC; the Default one should be the
		// per-NIC value as well.
		nicData := []network.ConfigData{
			{
				ProxName: infrav1.DefaultNetworkDevice,
				IPConfigs: []network.IPConfig{
					{IPAddress: netip.MustParsePrefix("10.10.0.99/24"), Default: false},
					{IPAddress: netip.MustParsePrefix("10.10.0.5/24"), Default: true},
				},
				Routes: []network.RoutingData{
					{To: netip.MustParsePrefix("0.0.0.0/0"), Via: netip.MustParseAddr("10.10.0.1")},
				},
			},
		}
		in := baseInput()
		in.PopulateNetworkAddresses(nicData)
		require.Equal(t, "10.10.0.5", in.IPv4)
		require.Equal(t, "10.10.0.5", in.NetworkAddresses[0].IPv4)
	})

	t.Run("VRFEntriesAreSkipped", func(t *testing.T) {
		nicData := []network.ConfigData{
			{
				ProxName: infrav1.DefaultNetworkDevice,
				IPConfigs: []network.IPConfig{
					{IPAddress: netip.MustParsePrefix("10.10.0.5/24"), Default: true},
				},
			},
			{
				// VRF entries have no ProxName.
				Type: network.TypeVRF,
				Name: "vrf-mgmt",
			},
		}
		in := baseInput()
		in.PopulateNetworkAddresses(nicData)
		require.Len(t, in.NetworkAddresses, 1)
		require.Equal(t, "net0", in.NetworkAddresses[0].DeviceName)
	})

	t.Run("NICWithNoIPsIsSkipped", func(t *testing.T) {
		nicData := []network.ConfigData{
			{
				ProxName:  "net0",
				IPConfigs: nil,
			},
			{
				ProxName: "net1",
				IPConfigs: []network.IPConfig{
					{IPAddress: netip.MustParsePrefix("10.20.0.5/24")},
				},
			},
		}
		in := baseInput()
		in.PopulateNetworkAddresses(nicData)
		require.Len(t, in.NetworkAddresses, 1)
		require.Equal(t, "net1", in.NetworkAddresses[0].DeviceName)
	})
}
