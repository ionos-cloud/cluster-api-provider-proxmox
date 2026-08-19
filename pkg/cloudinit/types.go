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
	"strconv"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

const (
	// FormatCloudConfig is the format for cloud-config.
	FormatCloudConfig = "cloud-config"
)

// MetadataInput is the public input to NewMetadata. Fields map 1:1 to the keys
// emitted in cloud-init meta-data.
type MetadataInput struct {
	InstanceID          string
	Hostname            string
	KubernetesVersion   string
	ProviderIDInjection bool

	// IPv4, IPv4Prefix, and IPv4Gateway describe the IPv4 default-gateway IP
	// of the VM (the address allocated from the cluster's default IPv4 pool on
	// the network device flagged as the default IPv4 device). Empty if the VM
	// has no IPv4 default-gateway IP.
	IPv4        string
	IPv4Prefix  string
	IPv4Gateway string

	// IPv6, IPv6Prefix, and IPv6Gateway are the IPv6 counterparts.
	IPv6        string
	IPv6Prefix  string
	IPv6Gateway string

	// NetworkAddresses holds per-device IP entries keyed by Proxmox device name.
	NetworkAddresses []NetworkAddress
}

// NetworkAddress describes the IP allocation for a single Proxmox network device.
// Rendered into cloud-init meta-data as keys suffixed by DeviceName, e.g.
// ipv4_net0, ipv6_prefix_net1.
type NetworkAddress struct {
	DeviceName  string
	IPv4        string
	IPv4Prefix  string
	IPv4Gateway string
	IPv6        string
	IPv6Prefix  string
	IPv6Gateway string
}

// BaseCloudInitData is shared across all the various types of files written to
// disk. It embeds MetadataInput (consumed by the metadata renderer) and adds
// NetworkConfigData (consumed by the network renderer). Either renderer leaves
// the other's fields zero-valued.
type BaseCloudInitData struct {
	MetadataInput
	NetworkConfigData []network.ConfigData
}

// PopulateNetworkAddresses fills the IP-related fields of in from nicData.
//
// Default-NIC alias fields (IPv4, IPv4Prefix, IPv4Gateway and the IPv6
// equivalents) are populated from the IPConfig flagged as the cluster default
// gateway (IPConfig.Default == true). Per-device entries are appended to
// NetworkAddresses for every device with at least one IP, keyed by the
// Proxmox device name (ProxName). Within a device, an IP with Default == true
// wins; otherwise the first IP of each family is used.
//
// The gateway values are taken from the device's default route (the route
// whose destination is the family's default prefix), since IP configuration in
// the network model no longer carries a per-address gateway.
//
// Entries without a ProxName (e.g. VRF virtual devices) are skipped.
func (in *MetadataInput) PopulateNetworkAddresses(nicData []network.ConfigData) {
	for _, nic := range nicData {
		if nic.ProxName == "" {
			continue
		}

		v4Gateway := defaultGateway(nic.Routes, true)
		v6Gateway := defaultGateway(nic.Routes, false)

		addr := NetworkAddress{DeviceName: string(nic.ProxName)}
		for _, ipc := range nic.IPConfigs {
			address := ipc.IPAddress.Addr().String()
			prefix := strconv.Itoa(ipc.IPAddress.Bits())
			if ipc.IPAddress.Addr().Is4() {
				if ipc.Default || addr.IPv4 == "" {
					addr.IPv4 = address
					addr.IPv4Prefix = prefix
					addr.IPv4Gateway = v4Gateway
				}
				if ipc.Default {
					in.IPv4 = address
					in.IPv4Prefix = prefix
					in.IPv4Gateway = v4Gateway
				}
			} else {
				if ipc.Default || addr.IPv6 == "" {
					addr.IPv6 = address
					addr.IPv6Prefix = prefix
					addr.IPv6Gateway = v6Gateway
				}
				if ipc.Default {
					in.IPv6 = address
					in.IPv6Prefix = prefix
					in.IPv6Gateway = v6Gateway
				}
			}
		}
		if addr.IPv4 != "" || addr.IPv6 != "" {
			in.NetworkAddresses = append(in.NetworkAddresses, addr)
		}
	}
}

// defaultGateway returns the Via address of the device's default route for the
// requested address family (is4 selects IPv4, otherwise IPv6), or "" if the
// device has no default route for that family. A default route is one whose
// destination prefix length is zero (0.0.0.0/0 or ::/0).
func defaultGateway(routes []network.RoutingData, is4 bool) string {
	for _, r := range routes {
		if r.Via.IsValid() && r.To.Bits() == 0 && r.To.Addr().Is4() == is4 {
			return r.Via.String()
		}
	}
	return ""
}
