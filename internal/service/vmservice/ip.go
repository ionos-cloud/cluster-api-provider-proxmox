/*
Copyright 2023 IONOS Cloud.

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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcileIPAddresses(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if machineScope.ProxmoxMachine.Status.IPAddresses != nil {
		// skip machine has IpAddress already.
		return false, nil
	}
	machineScope.Logger.V(4).Info("reconciling IPAddresses.")
	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForStaticIPAllocationReason, clusterv1.ConditionSeverityInfo, "")

	addresses := make(map[string]infrav1.IPAddress)

	// default device.
	if requeue, err = handleDefaultDevice(ctx, machineScope, addresses); err != nil || requeue {
		return true, errors.Wrap(err, "unable to handle default device")
	}

	if machineScope.ProxmoxMachine.Spec.Network != nil {
		if requeue, err = handleAdditionalDevices(ctx, machineScope, addresses); err != nil || requeue {
			return true, errors.Wrap(err, "unable to handle additional devices")
		}
	}

	// update the status.IpAddr.
	machineScope.Logger.V(4).Info("updating ProxmoxMachine.status.ipAddresses.")
	machineScope.ProxmoxMachine.Status.IPAddresses = addresses

	return true, nil
}

func findIPAddress(ctx context.Context, machineScope *scope.MachineScope, device string) (*ipamv1.IPAddress, error) {
	key := client.ObjectKey{
		Namespace: machineScope.Namespace(),
		Name:      formatIPAddressName(machineScope.Name(), device),
	}
	return machineScope.IPAMHelper.GetIPAddress(ctx, key)
}

func formatIPAddressName(name, device string) string {
	return fmt.Sprintf("%s-%s", name, device)
}

func machineHasIPAddress(machine *infrav1.ProxmoxMachine) bool {
	return machine.Status.IPAddresses[infrav1.DefaultNetworkDevice] != (infrav1.IPAddress{})
}

func handleIPAddressForDevice(ctx context.Context, machineScope *scope.MachineScope, device, format string, ipamRef *corev1.TypedLocalObjectReference) (string, error) {
	suffix := infrav1.DefaultSuffix
	if format == infrav1.IPV6Format {
		suffix += "6"
	}
	formattedDevice := fmt.Sprintf("%s-%s", device, suffix)
	ipAddr, err := findIPAddress(ctx, machineScope, formattedDevice)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return "", err
		}
		machineScope.Logger.V(4).Info("IPAddress not found, creating it.", "device", device)
		// IpAddress not yet created.
		err = machineScope.IPAMHelper.CreateIPAddressClaim(ctx, machineScope.ProxmoxMachine, device, format, machineScope.InfraCluster.Cluster.GetName(), ipamRef)
		if err != nil {
			return "", errors.Wrapf(err, "unable to create Ip address claim for machine %s", machineScope.Name())
		}
		return "", nil
	}

	ip := ipAddr.Spec.Address

	machineScope.Logger.V(4).Info("IPAddress found, ", "ip", ip, "device", device)

	// format ipTag as `ip_net0_<ipv4/6-address>`
	// to add it to the VM.
	ipTag := fmt.Sprintf("ip_%s_%s", device, ip)

	// Add ip tag if the Virtual Machine doesn't have it.
	if vm := machineScope.VirtualMachine; device == infrav1.DefaultNetworkDevice && !vm.HasTag(ipTag) && isIPV4(ip) {
		machineScope.Logger.V(4).Info("adding virtual machine ip tag.")
		t, err := machineScope.InfraCluster.ProxmoxClient.TagVM(ctx, vm, ipTag)
		if err != nil {
			return "", errors.Wrapf(err, "unable to add Ip tag to VirtualMachine %s", machineScope.Name())
		}
		machineScope.ProxmoxMachine.Status.TaskRef = ptr.To(string(t.UPID))
		return "", nil
	}

	return ip, nil
}

func handleDefaultDevice(ctx context.Context, machineScope *scope.MachineScope, addresses map[string]infrav1.IPAddress) (bool, error) {
	// check if the default network device is specified.
	var networkDevice *infrav1.NetworkDevice
	if machineScope.ProxmoxMachine.Spec.Network != nil && machineScope.ProxmoxMachine.Spec.Network.Default != nil {
		networkDevice = machineScope.ProxmoxMachine.Spec.Network.Default
	}
	// default network device ipv4.
	if machineScope.InfraCluster.ProxmoxCluster.Spec.IPv4Config != nil {
		if hasDHCPEnabled(machineScope.InfraCluster.ProxmoxCluster.Spec.IPv4Config,
			networkDevice,
			infrav1.DefaultNetworkDevice,
			infrav1.IPV4Format) {
			// skip device ipv4 has DHCP enabled.
			addresses[infrav1.DefaultNetworkDevice] = infrav1.IPAddress{
				IPV4: "DHCP",
			}
		} else {
			ip, err := handleIPAddressForDevice(ctx, machineScope, infrav1.DefaultNetworkDevice, infrav1.IPV4Format, nil)
			if err != nil || ip == "" {
				return true, err
			}
			addresses[infrav1.DefaultNetworkDevice] = infrav1.IPAddress{
				IPV4: ip,
			}
		}
	}

	// default network device ipv6.
	if machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config != nil {
		if hasDHCPEnabled(machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config,
			networkDevice,
			infrav1.DefaultNetworkDevice,
			infrav1.IPV6Format) {
			// skip device ipv6 has DHCP enabled.
			addr := addresses[infrav1.DefaultNetworkDevice]
			addr.IPV6 = "DHCP"
			addresses[infrav1.DefaultNetworkDevice] = addr
		} else {
			ip, err := handleIPAddressForDevice(ctx, machineScope, infrav1.DefaultNetworkDevice, infrav1.IPV6Format, nil)
			if err != nil || ip == "" {
				return true, err
			}
			addr := addresses[infrav1.DefaultNetworkDevice]
			addr.IPV6 = ip
			addresses[infrav1.DefaultNetworkDevice] = addr
		}
	}
	return false, nil
}

// nolint
func handleAdditionalDevices(ctx context.Context, machineScope *scope.MachineScope, addresses map[string]infrav1.IPAddress) (bool, error) {
	// additional network devices.
	for _, net := range machineScope.ProxmoxMachine.Spec.Network.AdditionalDevices {
		if net.IPv4PoolRef != nil {
			// additionalDevices don't rely on the cluster network configuration.
			// so we need to check if the device has DHCP enabled only.
			if net.DHCP4 {
				// skip device ipv4 has DHCP enabled.
				addresses[net.Name] = infrav1.IPAddress{
					IPV4: "DHCP",
				}
			} else {
				ip, err := handleIPAddressForDevice(ctx, machineScope, net.Name, infrav1.IPV4Format, net.IPv4PoolRef)
				if err != nil || ip == "" {
					return true, errors.Wrapf(err, "unable to handle IPAddress for device %s", net.Name)
				}

				addresses[net.Name] = infrav1.IPAddress{
					IPV4: ip,
				}
			}
		}

		if net.IPv6PoolRef != nil {
			// additionalDevices don't rely on the cluster network configuration.
			// so we need to check if the device has DHCP enabled only.
			if net.DHCP6 {
				// skip device ipv4 has DHCP enabled.
				addresses[net.Name] = infrav1.IPAddress{
					IPV6: "DHCP",
				}
				return false, nil
			} else {
				ip, err := handleIPAddressForDevice(ctx, machineScope, net.Name, infrav1.IPV6Format, net.IPv6PoolRef)
				if err != nil || ip == "" {
					return true, errors.Wrapf(err, "unable to handle IPAddress for device %s", net.Name)
				}

				addresses[net.Name] = infrav1.IPAddress{
					IPV6: ip,
				}
			}
		}
	}

	return false, nil
}

func isIPV4(ip string) bool {
	return netip.MustParseAddr(ip).Is4()
}

func hasDHCPEnabled(config *infrav1.IPConfig, device *infrav1.NetworkDevice, deviceName, format string) bool {
	switch {
	case deviceName == infrav1.DefaultNetworkDevice:
		if format == infrav1.IPV4Format {
			return (config != nil && config.DHCP) || (device != nil && device.DHCP4)
		} else if format == infrav1.IPV6Format {
			return (config != nil && config.DHCP) || (device != nil && device.DHCP6)
		}
	default:
		// additionalDevices don't rely on the cluster network configuration.
		if format == infrav1.IPV4Format {
			return device != nil && device.DHCP4
		} else if format == infrav1.IPV6Format {
			return device != nil && device.DHCP6
		}
	}
	return false
}
