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
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

const (
	// DefaultNetworkDeviceIPv4 is the default network device name for ipv4.
	DefaultNetworkDeviceIPv4 = "net0-inet"

	// DefaultNetworkDeviceIPv6 is the default network device name for ipv6.
	DefaultNetworkDeviceIPv6 = "net0-inet6"
)

// GetInClusterIPPoolRefs gets only the object references (per zone) from the clusters
// inClusterIPPools.
func GetInClusterIPPoolRefs(ctx context.Context, machineScope *scope.MachineScope) (struct {
	IPv4 *corev1.TypedLocalObjectReference
	IPv6 *corev1.TypedLocalObjectReference
}, error) {
	var ret struct {
		IPv4 *corev1.TypedLocalObjectReference
		IPv6 *corev1.TypedLocalObjectReference
	}

	pools, err := machineScope.IPAMHelper.GetInClusterPools(ctx, machineScope.ProxmoxMachine)
	if err != nil {
		return ret, err
	}

	if pools.IPv4 != nil {
		ret.IPv4 = &pools.IPv4.PoolRef
	}
	if pools.IPv6 != nil {
		ret.IPv6 = &pools.IPv6.PoolRef
	}

	return ret, nil
}

func extractUUID(input string) string {
	pattern := `(^|,)uuid=([0-9a-fA-F-]+)`

	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(input)

	if len(match) > 1 {
		if parsed, err := uuid.Parse(match[len(match)-1]); err == nil {
			return parsed.String()
		}
	}
	return ""
}

// IPAddressWithPrefix return formatted IP Address with prefix.
func IPAddressWithPrefix(ip string, prefix int) string {
	if ip == "" {
		return ""
	}
	return fmt.Sprintf("%s/%d", ip, prefix)
}

// extractNetworkModel returns the model out of net device input e.g. virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500.
func extractNetworkModel(input string) string {
	re := regexp.MustCompile(`([^,=]+)(?:=[^,]*)?,bridge=([^,]+)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractNetworkBridge returns the bridge out of net device input e.g. virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500.
func extractNetworkBridge(input string) string {
	re := regexp.MustCompile(`bridge=(\w+)`)
	match := re.FindStringSubmatch(input)
	if len(match) > 1 {
		return match[1]
	}
	return "unknown"
}

// extractNetworkMTU returns the mtu out of net device input e.g. virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500.
func extractNetworkMTU(input string) int32 {
	re := regexp.MustCompile(`mtu=(\d+)`)
	match := re.FindStringSubmatch(input)
	if len(match) > 1 {
		mtu, err := strconv.ParseInt(match[1], 10, 32)
		if err != nil {
			return 0
		}
		return int32(mtu)
	}

	return 0
}

// extractNetworkVLAN returns the vlan out of net device input e.g. virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,tag=100.
func extractNetworkVLAN(input string) int32 {
	re := regexp.MustCompile(`tag=(\d+)`)
	match := re.FindStringSubmatch(input)
	if len(match) > 1 {
		vlan, err := strconv.ParseInt(match[1], 10, 32)
		if err != nil {
			return 0
		}
		return int32(vlan)
	}

	return 0
}

// extractNetworkQueue returns the queue out of net device input e.g. virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,tag=100,queues=4.
func extractNetworkQueue(input string) int32 {
	re := regexp.MustCompile(`queues=(\d+)`)
	match := re.FindStringSubmatch(input)
	if len(match) > 1 {
		queue, err := strconv.ParseUint(match[1], 10, 16)
		if err != nil {
			return 0
		}
		return int32(queue)
	}

	return 0
}

func shouldUpdateNetworkDevices(machineScope *scope.MachineScope) bool {
	if machineScope.ProxmoxMachine.Spec.Network == nil {
		// no network config needed
		return false
	}

	nets := machineScope.VirtualMachine.VirtualMachineConfig.MergeNets()

	devices := machineScope.ProxmoxMachine.Spec.Network.NetworkDevices
	for _, v := range devices {
		name := v.Name
		if len(name) == 0 {
			name = infrav1.DefaultNetworkDevice
		}
		net := nets[string(name)]
		// device is empty.
		if len(net) == 0 {
			return true
		}

		model := extractNetworkModel(net)
		bridge := extractNetworkBridge(net)

		// current is different from the desired spec.
		if model != ptr.Deref(v.Model, "virtio") || bridge != ptr.Deref(v.Bridge, "") {
			return true
		}

		if v.MTU != nil {
			mtu := extractNetworkMTU(net)

			if mtu != *v.MTU {
				return true
			}
		}

		if v.VLAN != nil {
			vlan := extractNetworkVLAN(net)

			if vlan != *v.VLAN {
				return true
			}
		}

		if v.Queues != nil {
			queues := extractNetworkQueue(net)

			if queues != *v.Queues {
				return true
			}
		}
	}

	return false
}

// formatNetworkDevice formats a network device config
// example 'virtio,bridge=vmbr0,tag=100,queues=4'.
func formatNetworkDevice(model, bridge string, mtu *int32, vlan *int32, queues *int32) string {
	var components = []string{model, fmt.Sprintf("bridge=%s", bridge)}

	if mtu != nil {
		components = append(components, fmt.Sprintf("mtu=%d", *mtu))
	}

	if vlan != nil {
		components = append(components, fmt.Sprintf("tag=%d", *vlan))
	}

	if queues != nil {
		components = append(components, fmt.Sprintf("queues=%d", *queues))
	}

	return strings.Join(components, ",")
}

// extractMACAddress returns the macaddress out of net device input e.g. virtio=A6:23:64:4D:84:CB,bridge=vmbr1.
func extractMACAddress(input string) string {
	re := regexp.MustCompile(`=([^,]+),bridge`)
	matches := re.FindStringSubmatch(input)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

// NetNameToOffset converts a proxmox network name to a NetworkDevice offset.
func NetNameToOffset(name infrav1.NetName) (int, error) {
	offset, found := strings.CutPrefix(string(name), "net")
	if !found {
		return -1, fmt.Errorf("invalid proxmox network name: %s", name)
	}

	return strconv.Atoi(offset)
}

// OffsetToNetName converts an integer to a proxmox network name.
func OffsetToNetName(offset uint8) infrav1.NetName {
	return infrav1.NetName(fmt.Sprintf("net%d", offset))
}

// parseRouteTarget parses a route or FIB-rule target.
// A bare IP address yields its smallest enclosing prefix (/32 or /128).
// This is important for normalization of routes (route metric conflicts).
// "default" and "all" yield 0.0.0.0/0 (or ::/0 when is6 is true). A nil or
// empty input yields the zero prefix to signal "unset".
func parseRouteTarget(s *string, is6 *bool) (netip.Prefix, error) {
	if ptr.Deref(s, "") == "" {
		return netip.Prefix{}, nil
	}
	if network.IsRouteTargetPlaceholder(s) {
		if ptr.Deref(is6, false) {
			return netip.PrefixFrom(netip.IPv6Unspecified(), 0), nil
		}
		return netip.PrefixFrom(netip.IPv4Unspecified(), 0), nil
	}
	if p, err := netip.ParsePrefix(*s); err == nil {
		return p, nil
	}
	addr, err := netip.ParseAddr(*s)
	if err != nil {
		return netip.Prefix{}, errors.Wrapf(err, "invalid target %q", *s)
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// resolveFamily determines the address family used to expand a placeholder
// target. An explicit is6 always wins; otherwise the family is borrowed from the
// first hint that carries a concrete address (a bare IP or a prefix).
func resolveFamily(is6 *bool, hints ...*string) *bool {
	if is6 != nil {
		return is6
	}
	for _, h := range hints {
		if ptr.Deref(h, "") == "" || network.IsRouteTargetPlaceholder(h) {
			continue
		}
		if p, err := netip.ParsePrefix(*h); err == nil {
			return ptr.To(p.Addr().Is6())
		}
		if addr, err := netip.ParseAddr(*h); err == nil {
			return ptr.To(addr.Is6())
		}
	}
	return nil
}

// parseVia parses a route Via. It must be a bare IP address; prefixes and
// placeholders are rejected. A nil or empty input yields the zero address.
func parseVia(s *string) (netip.Addr, error) {
	if s == nil || *s == "" {
		return netip.Addr{}, nil
	}
	addr, err := netip.ParseAddr(*s)
	if err != nil {
		return netip.Addr{}, errors.Wrapf(err, "invalid via %q", *s)
	}
	return addr, nil
}

// ToRoutingData converts a slice of infrav1.RouteSpec into renderer-side
// RoutingData, validating that the address fields parse.
func ToRoutingData(specs []infrav1.RouteSpec) ([]network.RoutingData, error) {
	out := make([]network.RoutingData, 0, len(specs))
	for _, spec := range specs {
		// A "default"/"all" placeholder To needs an address family. Prefer an
		// explicit is6, otherwise borrow it from the Via gateway.
		to, err := parseRouteTarget(spec.To, resolveFamily(spec.Is6, spec.Via))
		if err != nil {
			return nil, errors.Wrap(err, "invalid route")
		}
		via, err := parseVia(spec.Via)
		if err != nil {
			return nil, err
		}
		out = append(out, network.RoutingData{
			To:     to,
			Table:  spec.Table,
			Via:    via,
			Metric: spec.Metric,
		})
	}
	return out, nil
}

// ToFIBRuleData converts a slice of infrav1.RoutingPolicySpec into
// renderer-side FIBRuleData, validating that the address fields parse.
func ToFIBRuleData(specs []infrav1.RoutingPolicySpec) ([]network.FIBRuleData, error) {
	out := make([]network.FIBRuleData, 0, len(specs))
	for _, spec := range specs {
		// A "default"/"all" placeholder To/From needs an address family. Prefer
		// an explicit is6, otherwise borrow it from whichever of To/From is a
		// concrete address (a FIB rule cannot mix families).
		is6 := resolveFamily(spec.Is6, spec.To, spec.From)
		to, err := parseRouteTarget(spec.To, is6)
		if err != nil {
			return nil, errors.Wrap(err, "invalid FIB rule")
		}
		// From accepts the same target syntax as To.
		from, err := parseRouteTarget(spec.From, is6)
		if err != nil {
			return nil, errors.Wrap(err, "invalid FIB rule from")
		}
		out = append(out, network.FIBRuleData{
			To:       to,
			Table:    spec.Table,
			From:     from,
			Priority: spec.Priority,
		})
	}
	return out, nil
}
