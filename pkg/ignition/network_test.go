package ignition

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
)

var (
	expectedValidNetworkConfig = map[string][]byte{
		"00-eth0.network": []byte(`[Match]
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
`),
		"01-eth1.network": []byte(`[Match]
MACAddress=E2:8E:95:1F:EB:36

[Network]
DNS=10.0.1.1
[Address]
Address=10.0.1.84/25

[Route]
Destination=0.0.0.0/0
Gateway=10.0.1.1
Metric=200
`),
	}

	expectedValidNetworkConfigWithVRF = map[string][]byte{
		"00-vrf0.netdev": []byte(`[NetDev]
Name=vrf0
Kind=vrf

[VRF]
Table=644
`),
		"00-eth0.network": []byte(`[Match]
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
`),
		"01-eth1.network": []byte(`[Match]
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
`),

		"02-vrf2.network": []byte(`[Match]
Name=vrf0

[Route]
Destination=3.4.5.6
Gateway=10.0.1.1
Metric=100
`),
	}
)

func TestRenderNetworkConfigData(t *testing.T) {
	type args struct {
		nics []cloudinit.NetworkConfigData
	}

	type want struct {
		units map[string][]byte
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
				nics: []cloudinit.NetworkConfigData{
					{
						Type:        "ethernet",
						Name:        "eth0",
						MacAddress:  "E2:B8:FE:E7:50:75",
						IPAddress:   "10.0.0.98/25",
						Gateway:     "10.0.0.1",
						ProxName:    "net0",
						DNSServers:  []string{"10.0.1.1"},
						Metric:      ptr.To(uint32(100)),
						IPV6Address: "2001:db8:1::10/64",
						Gateway6:    "2001:db8:1::1",
					},
					{
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "E2:8E:95:1F:EB:36",
						IPAddress:  "10.0.1.84/25",
						Gateway:    "10.0.1.1",
						ProxName:   "net1",
						DNSServers: []string{"10.0.1.1"},
						Metric:     ptr.To(uint32(200)),
					},
				},
			},
			want: want{
				units: expectedValidNetworkConfig,
				err:   nil,
			},
		},
		"ValidNetworkdConfigWithVRF": {
			reason: "render valid networkd with static ip and VRF",
			args: args{
				nics: []cloudinit.NetworkConfigData{
					{
						Type:        "ethernet",
						Name:        "eth0",
						MacAddress:  "E2:B8:FE:E7:50:75",
						IPAddress:   "10.0.0.98/25",
						Gateway:     "10.0.0.1",
						ProxName:    "net0",
						DNSServers:  []string{"10.0.1.1"},
						Metric:      ptr.To(uint32(100)),
						IPV6Address: "2001:db8:1::10/64",
						Gateway6:    "2001:db8:1::1",
					},
					{
						Type:       "ethernet",
						Name:       "eth1",
						MacAddress: "E2:8E:95:1F:EB:36",
						IPAddress:  "10.0.1.84/25",
						Gateway:    "10.0.1.1",
						ProxName:   "net1",
						DNSServers: []string{"10.0.1.1"},
						Metric:     ptr.To(uint32(200)),
					},
					{
						Type:       "vrf",
						Name:       "vrf0",
						ProxName:   "net1",
						Table:      644,
						Interfaces: []string{"eth1"},
						Routes: []cloudinit.RoutingData{{
							To:     "3.4.5.6",
							Via:    "10.0.1.1",
							Metric: 100,
						}},
					},
				},
			},
			want: want{
				units: expectedValidNetworkConfigWithVRF,
				err:   nil,
			},
		},
	}

	for n, tc := range cases {
		t.Run(n, func(t *testing.T) {
			units, err := RenderNetworkConfigData(cloudinit.BaseCloudInitData{NetworkConfigData: tc.args.nics})
			require.ErrorIs(t, err, tc.want.err)
			for k := range units {
				require.Equal(t, tc.want.units[k], units[k])
			}
		})
	}
}
