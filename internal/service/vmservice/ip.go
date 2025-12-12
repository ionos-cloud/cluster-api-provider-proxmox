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
	"slices"
	"strconv"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcileIPAddresses(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) != infrav1.WaitingForStaticIPAllocationReason {
		// Machine is in the wrong state to reconcile, we only reconcile VMs Waiting for IP Address assignment
		return false, nil
	}

	machineScope.Logger.V(4).Info("reconciling IPAddresses.")

	// TODO: This datastructure is BAD
	netPoolAddresses := make(map[string]map[string][]string)

	if machineScope.ProxmoxMachine.Spec.Network != nil {
		if requeue, err = handleDevices(ctx, machineScope, netPoolAddresses); err != nil || requeue {
			if err == nil {
				return true, errors.Wrap(err, "requeuing network reconcillation")
			} else {
				return true, errors.Wrap(err, "unable to handle network devices")
			}
		}
	}

	defaultDevicePools := netPoolAddresses[infrav1.DefaultNetworkDevice]
	defaultPools := machineScope.InfraCluster.ProxmoxCluster.Status.InClusterIPPoolRef
	for _, pool := range defaultPools {
		if len(defaultDevicePools[pool.Name]) < 1 {
			continue
		}
		ip := defaultDevicePools[pool.Name][0]
		// format ipTag as `ip_net0_<ipv4/6-address>`
		// to add it to the VM.
		ipTag := fmt.Sprintf("ip_%s_%s", infrav1.DefaultNetworkDevice, ip)

		// TODO: the requeuing logic is wrong. we only want to print the default pools
		requeue := false

		// Todo: add tagging to its own stage
		// Add ip tag if the Virtual Machine doesn't have it.
		if vm := machineScope.VirtualMachine; !vm.HasTag(ipTag) && isIPV4(ip) {
			machineScope.Logger.V(4).Info("adding virtual machine ip tag.")
			t, err := machineScope.InfraCluster.ProxmoxClient.TagVM(ctx, vm, ipTag)
			if err != nil {
				return false, errors.Wrapf(err, "unable to add Ip tag to VirtualMachine %s", machineScope.Name())
			}
			machineScope.ProxmoxMachine.Status.TaskRef = ptr.To(string(t.UPID))
			requeue = true
		}
		if requeue {
			// send the machine to requeue so promoxclient can execute
			return true, nil
		}
	}

	// update the status.IpAddr.

	// TODO: This datastructure should be redundant. Too many loops too
	statusAddresses := make(map[string]*infrav1.IPAddresses, len(netPoolAddresses))
	for net, pools := range netPoolAddresses {
		for _, ips := range pools {
			for _, ip := range ips {
				if _, e := statusAddresses[net]; !e {
					statusAddresses[net] = new(infrav1.IPAddresses)
				}
				if isIPV4(ip) {
					statusAddresses[net].IPV4 = append(statusAddresses[net].IPV4, ip)
				} else {
					statusAddresses[net].IPV6 = append(statusAddresses[net].IPV6, ip)
				}
			}
		}
	}
	machineScope.Logger.V(4).Info("updating ProxmoxMachine.status.ipAddresses.")
	machineScope.ProxmoxMachine.Status.IPAddresses = statusAddresses

	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForBootstrapDataReconcilationReason, clusterv1.ConditionSeverityInfo, "")

	return true, nil
}

// Todo: This function is only called in a helper
func formatIPAddressName(name, pool, device string) string {
	return fmt.Sprintf("%s-%s-%s", name, pool, device)
}

// findIPAddress returns all IPAddresses owned by a pool and a machine
func findIPAddress(ctx context.Context, poolRef *corev1.TypedLocalObjectReference, machineScope *scope.MachineScope) ([]ipamv1.IPAddress, error) {
	return machineScope.IPAMHelper.GetIPAddressV2(ctx, *poolRef, machineScope.ProxmoxMachine)
}

// findIPAddressesByPool attempts to return all ip addresses belonging to a device.
func findIPAddressesByPool(ctx context.Context, machineScope *scope.MachineScope, device string, poolRef corev1.TypedLocalObjectReference) ([]ipamv1.IPAddress, error) {
	addresses, err := machineScope.IPAMHelper.GetIPAddressByPool(ctx, poolRef)
	if err != nil {
		return nil, err
	}

	var out []ipamv1.IPAddress
	for _, a := range addresses {
		// if strings.Contains(a.Name, machineScope.Name()+device) {
		out = append(out, a)
		// }
	}
	return out, nil
}

func findIPAddressGatewayMetric(ctx context.Context, machineScope *scope.MachineScope, ipAddress *ipamv1.IPAddress) (*int32, error) {
	annotations, err := machineScope.IPAMHelper.GetIPPoolAnnotations(ctx, ipAddress)
	if err != nil {
		return nil, err
	}
	var rv *int32

	if s, exists := annotations["metric"]; exists {
		metric, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return nil, err
		}
		rv = ptr.To(int32(metric))
	}
	return rv, nil
}

func machineHasIPAddress(machine *infrav1.ProxmoxMachine) bool {
	// Every machine needs to have at least one IPv4 or IPv6 host network address
	if machine.Status.IPAddresses[infrav1.DefaultNetworkDevice] == nil {
		return false
	} else {
		return len(machine.Status.IPAddresses[infrav1.DefaultNetworkDevice].IPV4) > 0 ||
			len(machine.Status.IPAddresses[infrav1.DefaultNetworkDevice].IPV6) > 0
	}
}

func handleIPAddresses(ctx context.Context, machineScope *scope.MachineScope, dev *string, poolNum int, poolRef *corev1.TypedLocalObjectReference) ([]string, error) {
	device := ptr.Deref(dev, infrav1.DefaultNetworkDevice)

	ipAddresses, err := findIPAddress(ctx, poolRef, machineScope)
	if err != nil {
		// Technically this error can not occure, as fieldselectors just return empty lists
		if !apierrors.IsNotFound(err) {
			return []string{}, err
		}
	}

	if len(ipAddresses) == 0 {
		machineScope.Logger.V(4).Info("IPAddress not found, creating it.", "device", device)
		// IpAddress not yet created.
		err = machineScope.IPAMHelper.CreateIPAddressClaimV2(ctx, machineScope.ProxmoxMachine, device, poolNum, machineScope.InfraCluster.Cluster.GetName(), poolRef)
		if err != nil {
			return []string{}, errors.Wrapf(err, "unable to create Ip address claim for machine %s", machineScope.Name())
		}

		// send the machine to requeue so ipaddresses can be created
		return []string{}, nil
	}

	out := make([]string, 0)
	for _, ip := range ipAddresses {
		ip := ip.Spec.Address
		out = append(out, ip)
		machineScope.Logger.V(4).Info("IPAddress found, ", "ip", ip, "device", device)

	}

	return out, nil
}

func handleDevices(ctx context.Context, machineScope *scope.MachineScope, addresses map[string]map[string][]string) (bool, error) {
	for _, net := range machineScope.ProxmoxMachine.Spec.Network.NetworkDevices {
		// TODO: Where should prepending default clusterpools belong
		// TODO: Network Zones
		pools := []corev1.TypedLocalObjectReference{}
		if *net.Name == infrav1.DefaultNetworkDevice {
			poolsRef, err := GetInClusterIPPoolsFromMachine(ctx, machineScope)
			if err != nil {
				return false, err
			}
			pools = *poolsRef
		}
		for i, ipPool := range slices.Concat(pools, net.InterfaceConfig.IPPoolRef) {
			ipAddresses, err := handleIPAddresses(ctx, machineScope, net.Name, i, &ipPool)

			// requeue machine if tag or ipaddress need creation
			if len(ipAddresses) == 0 {
				return true, nil
			}

			for _, ip := range ipAddresses {
				if err != nil || ip == "" {
					fmt.Println("handleDevices", "err", err, "ip", ip)
					return true, errors.Wrapf(err, "unable to handle IPAddress for device %+v, pool %s", net.Name, ipPool.Name)
				}

				poolMap := addresses[*net.Name]
				if poolMap == nil {
					poolMap = make(map[string][]string)
				}

				poolIPAddresses := poolMap[ipPool.Name]

				poolIPAddresses = append(poolIPAddresses, ip)
				poolMap[ipPool.Name] = poolIPAddresses

				addresses[*net.Name] = poolMap
			}
		}
	}

	return false, nil
}

func isIPV4(ip string) bool {
	return netip.MustParseAddr(ip).Is4()
}
