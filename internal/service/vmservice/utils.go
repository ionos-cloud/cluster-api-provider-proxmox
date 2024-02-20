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

package vmservice

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/google/uuid"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

const (
	// DefaultNetworkDeviceIPV4 is the default network device name for ipv4.
	DefaultNetworkDeviceIPV4 = "net0-inet"

	// DefaultNetworkDeviceIPV6 is the default network device name for ipv6.
	DefaultNetworkDeviceIPV6 = "net0-inet6"
)

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
func extractNetworkMTU(input string) uint16 {
	re := regexp.MustCompile(`mtu=(\d+)`)
	match := re.FindStringSubmatch(input)
	if len(match) > 1 {
		mtu, err := strconv.ParseUint(match[1], 10, 16)
		if err != nil {
			return 0
		}
		return uint16(mtu)
	}

	return 0
}

func shouldUpdateNetworkDevices(machineScope *scope.MachineScope) bool {
	if machineScope.ProxmoxMachine.Spec.Network == nil {
		// no network config needed
		return false
	}

	nets := machineScope.VirtualMachine.VirtualMachineConfig.MergeNets()

	if machineScope.ProxmoxMachine.Spec.Network.Default != nil {
		net0 := nets[infrav1alpha1.DefaultNetworkDevice]
		if net0 == "" {
			return true
		}

		desiredDefault := *machineScope.ProxmoxMachine.Spec.Network.Default

		model := extractNetworkModel(net0)
		bridge := extractNetworkBridge(net0)

		if model != *desiredDefault.Model || bridge != desiredDefault.Bridge {
			return true
		}

		if desiredDefault.MTU != nil {
			mtu := extractNetworkMTU(net0)

			if mtu != *desiredDefault.MTU {
				return true
			}
		}
	}

	devices := machineScope.ProxmoxMachine.Spec.Network.AdditionalDevices
	for _, v := range devices {
		net := nets[v.Name]
		// device is empty.
		if len(net) == 0 {
			return true
		}

		model := extractNetworkModel(net)
		bridge := extractNetworkBridge(net)

		// current is different from the desired spec.
		if model != *v.Model || bridge != v.Bridge {
			return true
		}

		if v.MTU != nil {
			mtu := extractNetworkMTU(net)

			if mtu != *v.MTU {
				return true
			}
		}
	}

	return false
}

// formatNetworkDevice formats a network device config
// example 'virtio,bridge=vmbr0'.
func formatNetworkDevice(model, bridge string, mtu *uint16) string {
	if mtu == nil {
		return fmt.Sprintf("%s,bridge=%s", model, bridge)
	}

	return fmt.Sprintf("%s,bridge=%s,mtu=%d", model, bridge, *mtu)
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
