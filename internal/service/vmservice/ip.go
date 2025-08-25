/*
Copyright 2023-2025 IONOS Cloud.

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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1alpha2 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcileIPAddresses(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if machineScope.ProxmoxMachine.Status.IPAddresses != nil {
		// skip machine has IpAddress already.
		return false, nil
	}
	machineScope.Logger.V(4).Info("reconciling IPAddresses.")
	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition, infrav1alpha2.WaitingForStaticIPAllocationReason, clusterv1.ConditionSeverityInfo, "")

	addresses := make(map[string]string)

	if machineScope.ProxmoxMachine.Spec.Network != nil {
		// fmt.Println( handleDevices(ctx, machineScope, addresses))
		if requeue, err = handleDevices(ctx, machineScope, addresses); err != nil || requeue {
			return true, errors.Wrap(err, "unable to handle network devices")
		}
	}

	/*
		// default device.
		if requeue, err = handleDefaultDevice(ctx, machineScope, addresses); err != nil || requeue {
			return true, errors.Wrap(err, "unable to handle default device")
		}

		if machineScope.ProxmoxMachine.Spec.Network != nil {
			if requeue, err = handleAdditionalDevices(ctx, machineScope, addresses); err != nil || requeue {
				return true, errors.Wrap(err, "unable to handle additional devices")
			}
		}
	*/

	// update the status.IpAddr.

	statusAddresses := make(map[string]*infrav1alpha2.IPAddresses, len(addresses))
	for k, v := range addresses {
		if _, e := statusAddresses[k]; !e {
			statusAddresses[k] = new(infrav1alpha2.IPAddresses)
		}
		if isIPV4(v) {
			statusAddresses[k].IPV4 = append(statusAddresses[k].IPV4, v)
		} else {
			statusAddresses[k].IPV6 = append(statusAddresses[k].IPV6, v)
		}
	}
	machineScope.Logger.V(4).Info("updating ProxmoxMachine.status.ipAddresses.")
	machineScope.ProxmoxMachine.Status.IPAddresses = statusAddresses

	return true, nil
}

func findIPAddress(ctx context.Context, machineScope *scope.MachineScope, device string) (*ipamv1.IPAddress, error) {
	key := client.ObjectKey{
		Namespace: machineScope.Namespace(),
		Name:      formatIPAddressName(machineScope.Name(), device),
	}
	return machineScope.IPAMHelper.GetIPAddress(ctx, key)
}

// findIPAddressesByPool attempts to return all ip addresses belonging to a device.
func findIPAddressesByPool(ctx context.Context, machineScope *scope.MachineScope, device string, poolRef corev1.TypedLocalObjectReference) ([]ipamv1.IPAddress, error) {
	addresses, err := machineScope.IPAMHelper.GetIPAddressByPool(ctx, poolRef)
	if err != nil {
		return nil, err
	}

	var out []ipamv1.IPAddress
	for _, a := range addresses {
		if strings.Contains(a.Name, machineScope.Name()+device) {
			out = append(out, a)
		}
	}
	return out, nil
}

func findIPAddressGatewayMetric(ctx context.Context, machineScope *scope.MachineScope, ipAddress *ipamv1.IPAddress) (*uint32, error) {
	annotations, err := machineScope.IPAMHelper.GetIPPoolAnnotations(ctx, ipAddress)
	if err != nil {
		return nil, err
	}
	var rv *uint32

	if s, exists := annotations["metric"]; exists {
		metric, err := strconv.ParseUint(s, 0, 32)
		if err != nil {
			return nil, err
		}
		rv = ptr.To(uint32(metric))
	}
	return rv, nil
}

func formatIPAddressName(name, device string) string {
	return fmt.Sprintf("%s-%s", name, device)
}

func machineHasIPAddress(machine *infrav1alpha2.ProxmoxMachine) bool {
	// TODO: does this work?
	return machine.Status.IPAddresses[infrav1alpha2.DefaultNetworkDevice] != nil
}

/*
func handleIPAddressForDevice(ctx context.Context, machineScope *scope.MachineScope, device, format string, ipamRef *corev1.TypedLocalObjectReference) (string, error) {
	suffix := infrav1alpha2.DefaultSuffix
	if format == infrav1alpha2.IPV6Format {
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
	if vm := machineScope.VirtualMachine; device == infrav1alpha2.DefaultNetworkDevice && !vm.HasTag(ipTag) && isIPV4(ip) {
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
*/

/*
func handleDefaultDevice(ctx context.Context, machineScope *scope.MachineScope, addresses map[string]infrav1alpha2.IPAddress) (bool, error) {
	// default network device ipv4.
	if machineScope.InfraCluster.ProxmoxCluster.Spec.IPv4Config != nil ||
		(machineScope.ProxmoxMachine.Spec.Network != nil && machineScope.ProxmoxMachine.Spec.Network.Default.IPv4PoolRef != nil) {
		var ipamRef *corev1.TypedLocalObjectReference
		if machineScope.ProxmoxMachine.Spec.Network != nil && machineScope.ProxmoxMachine.Spec.Network.Default.IPv4PoolRef != nil {
			ipamRef = machineScope.ProxmoxMachine.Spec.Network.Default.IPv4PoolRef
		}

		ip, err := handleIPAddressForDevice(ctx, machineScope, infrav1alpha2.DefaultNetworkDevice, infrav1alpha2.IPV4Format, ipamRef)
		if err != nil || ip == "" {
			return true, err
		}
		addresses[infrav1alpha2.DefaultNetworkDevice] = infrav1alpha2.IPAddress{
			IPV4: ip,
		}
	}

	// default network device ipv6.
	if machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config != nil ||
		(machineScope.ProxmoxMachine.Spec.Network != nil && machineScope.ProxmoxMachine.Spec.Network.Default.IPv6PoolRef != nil) {
		var ipamRef *corev1.TypedLocalObjectReference
		if machineScope.ProxmoxMachine.Spec.Network != nil && machineScope.ProxmoxMachine.Spec.Network.Default.IPv6PoolRef != nil {
			ipamRef = machineScope.ProxmoxMachine.Spec.Network.Default.IPv6PoolRef
		}

		ip, err := handleIPAddressForDevice(ctx, machineScope, infrav1alpha2.DefaultNetworkDevice, infrav1alpha2.IPV6Format, ipamRef)
		if err != nil || ip == "" {
			return true, err
		}

		addr := addresses[infrav1alpha2.DefaultNetworkDevice]
		addr.IPV6 = ip
		addresses[infrav1alpha2.DefaultNetworkDevice] = addr
	}
	return false, nil
}
*/

func handleIPAddress(ctx context.Context, machineScope *scope.MachineScope, device string, poolNum int, ipamRef *corev1.TypedLocalObjectReference) (string, error) {
	suffix := infrav1alpha2.DefaultSuffix

	formattedDevice := fmt.Sprintf("%s%02d-%s", device, poolNum, suffix)

	ipAddr, err := findIPAddress(ctx, machineScope, formattedDevice)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return "", err
		}
		machineScope.Logger.V(4).Info("IPAddress not found, creating it.", "device", device)
		// IpAddress not yet created.
		err = machineScope.IPAMHelper.CreateIPAddressClaimV2(ctx, machineScope.ProxmoxMachine, device, poolNum, machineScope.InfraCluster.Cluster.GetName(), ipamRef)
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
	if vm := machineScope.VirtualMachine; device == infrav1alpha2.DefaultNetworkDevice && !vm.HasTag(ipTag) && isIPV4(ip) {
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

func handleDevices(ctx context.Context, machineScope *scope.MachineScope, addresses map[string]string) (bool, error) {
	// additional network devices.
	for _, net := range machineScope.ProxmoxMachine.Spec.Network.NetworkDevices {
		fmt.Println(net)
		for i, ipPool := range net.InterfaceConfig.IPPoolRef {
			// TODO: Unfuck this
			ip, err := handleIPAddress(ctx, machineScope, net.Name, i, &ipPool)
			if err != nil || ip == "" {
				fmt.Println("handleDevices", "err", err, "ip", ip)
				return true, errors.Wrapf(err, "unable to handle IPAddress for device %s, pool %s", net.Name, ipPool.Name)
			}

			addresses[net.Name+fmt.Sprint(i)] = ip
		}
		/*
			if net.IPv4PoolRef != nil {
				ip, err := handleIPAddressForDevice(ctx, machineScope, net.Name, infrav1alpha2.IPV4Format, net.IPv4PoolRef)
				if err != nil || ip == "" {
					return true, errors.Wrapf(err, "unable to handle IPAddress for device %s", net.Name)
				}

				addresses[net.Name] = infrav1alpha2.IPAddress{
					IPV4: ip,
				}
			}

			if net.IPv6PoolRef != nil {
				ip, err := handleIPAddressForDevice(ctx, machineScope, net.Name, infrav1alpha2.IPV6Format, net.IPv6PoolRef)
				if err != nil || ip == "" {
					return true, errors.Wrapf(err, "unable to handle IPAddress for device %s", net.Name)
				}

				addresses[net.Name] = infrav1alpha2.IPAddress{
					IPV6: ip,
				}
			}
		*/
	}

	return false, nil
}

/*
func handleAdditionalDevices(ctx context.Context, machineScope *scope.MachineScope, addresses map[string]infrav1alpha2.IPAddress) (bool, error) {
	// additional network devices.
	for _, net := range machineScope.ProxmoxMachine.Spec.Network.AdditionalDevices {
		if net.IPv4PoolRef != nil {
			ip, err := handleIPAddressForDevice(ctx, machineScope, net.Name, infrav1alpha2.IPV4Format, net.IPv4PoolRef)
			if err != nil || ip == "" {
				return true, errors.Wrapf(err, "unable to handle IPAddress for device %s", net.Name)
			}

			addresses[net.Name] = infrav1alpha2.IPAddress{
				IPV4: ip,
			}
		}

		if net.IPv6PoolRef != nil {
			ip, err := handleIPAddressForDevice(ctx, machineScope, net.Name, infrav1alpha2.IPV6Format, net.IPv6PoolRef)
			if err != nil || ip == "" {
				return true, errors.Wrapf(err, "unable to handle IPAddress for device %s", net.Name)
			}

			addresses[net.Name] = infrav1alpha2.IPAddress{
				IPV6: ip,
			}
		}
	}

	return false, nil
}
*/

func isIPV4(ip string) bool {
	return netip.MustParseAddr(ip).Is4()
}
