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
	"reflect"
	"slices"
	"strconv"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"         //nolint:staticcheck
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"            //nolint:staticcheck
	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions" //nolint:staticcheck

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcileIPAddresses(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) != infrav1.WaitingForStaticIPAllocationReason {
		// Machine is in the wrong state to reconcile, we only reconcile VMs Waiting for IP Address assignment
		return false, nil
	}

	machineScope.Logger.V(4).Info("reconciling IPAddresses.")

	// TODO: This datastructure is less bad, but still bad
	netPoolAddresses := make(map[string]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)

	if machineScope.ProxmoxMachine.Spec.Network != nil {
		if requeue, err = handleDevices(ctx, machineScope, netPoolAddresses); err != nil || requeue {
			if err == nil {
				return true, errors.Wrap(err, "requeuing network reconcillation")
			}
			return true, errors.Wrap(err, "unable to handle network devices")
		}
	}

	// TODO: move to own state machine stage. Doesn't belong here, really
	defaultDevicePools := netPoolAddresses[infrav1.DefaultNetworkDevice]
	defaultPools := machineScope.InfraCluster.ProxmoxCluster.Status.InClusterIPPoolRef
	for _, pool := range defaultPools {
		poolRef := corev1.TypedLocalObjectReference{
			Name:     pool.Name,
			Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
			APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		}
		for defaultPool, ipAddresses := range defaultDevicePools {
			if reflect.DeepEqual(defaultPool, poolRef) {
				// Todo: This is not necessarily the default IP.
				_, err := setVMIPAddressTag(ctx, machineScope, ipAddresses[0])
				if err != nil {
					return false, err
				}
			}
		}
	}

	// update status.IpAddr.
	// TODO: This datastructure should be redundant. Too many loops too
	statusAddresses := make(map[string]*infrav1.IPAddresses, len(netPoolAddresses))
	for net, pools := range netPoolAddresses {
		for _, ips := range pools {
			for _, ip := range ips {
				if _, e := statusAddresses[net]; !e {
					statusAddresses[net] = new(infrav1.IPAddresses)
				}
				if isIPv4(ip.Spec.Address) {
					statusAddresses[net].IPv4 = append(statusAddresses[net].IPv4, ip.Spec.Address)
				} else {
					statusAddresses[net].IPv6 = append(statusAddresses[net].IPv6, ip.Spec.Address)
				}
			}
		}
	}
	machineScope.Logger.V(4).Info("updating ProxmoxMachine.status.ipAddresses.")
	machineScope.ProxmoxMachine.Status.IPAddresses = statusAddresses

	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForBootstrapDataReconcilationReason, clusterv1.ConditionSeverityInfo, "")

	return true, nil
}

// Todo: add tagging to its own stage.
func setVMIPAddressTag(ctx context.Context, machineScope *scope.MachineScope, ipAddress ipamv1.IPAddress) (bool, error) {
	// format ipTag as `ip_net0_<ipv4/6-address>`
	// to add it to the VM.
	ipTag := fmt.Sprintf("ip_%s_%s", infrav1.DefaultNetworkDevice, ipAddress.Spec.Address)

	requeue := false
	// TODO: IPv6 tag?
	// Add ipv4 tag if the Virtual Machine doesn't have it.
	if vm := machineScope.VirtualMachine; !vm.HasTag(ipTag) && isIPv4(ipAddress.Spec.Address) {
		machineScope.Logger.V(4).Info("adding virtual machine ip tag.", "ip", ipAddress.Spec.Address)
		t, err := machineScope.InfraCluster.ProxmoxClient.TagVM(ctx, vm, ipTag)
		if err != nil {
			return false, errors.Wrapf(err, "unable to add Ip tag to VirtualMachine %s", machineScope.Name())
		}
		machineScope.ProxmoxMachine.Status.TaskRef = ptr.To(string(t.UPID))
		requeue = true
	}

	return requeue, nil
}

// findIPAddress returns all IPAddresses owned by a pool and a machine.
func findIPAddress(ctx context.Context, poolRef corev1.TypedLocalObjectReference, machineScope *scope.MachineScope) ([]ipamv1.IPAddress, error) {
	return machineScope.IPAMHelper.GetIPAddressV2(ctx, poolRef, machineScope.ProxmoxMachine)
}

func findIPAddressGatewayMetric(ctx context.Context, machineScope *scope.MachineScope, ipAddress *ipamv1.IPAddress) (*int32, error) {
	annotations, err := machineScope.IPAMHelper.GetIPPoolAnnotations(ctx, ipAddress)
	if err != nil {
		return nil, err
	}
	var rv *int32

	// Remove this codepath with the next api change (field has moved to infrav1.ProxmoxGatewayMetricAnnotation).
	if s, exists := annotations["metric"]; exists && len(s) > 0 {
		metric, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return nil, err
		}
		rv = ptr.To(int32(metric))
	}

	if s, exists := annotations[infrav1.ProxmoxGatewayMetricAnnotation]; exists && len(s) > 0 {
		metric, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return nil, err
		}
		rv = ptr.To(int32(metric))
	}

	return rv, nil
}

func handleIPAddresses(ctx context.Context, machineScope *scope.MachineScope, dev *string, poolNum int, poolRef corev1.TypedLocalObjectReference) ([]ipamv1.IPAddress, error) {
	device := ptr.Deref(dev, infrav1.DefaultNetworkDevice)

	ipAddresses, err := findIPAddress(ctx, poolRef, machineScope)
	if err != nil {
		// Technically this error cannot occur as fieldselectors just return empty lists
		if !apierrors.IsNotFound(err) {
			return []ipamv1.IPAddress{}, err
		}
	}

	if len(ipAddresses) == 0 {
		machineScope.Logger.V(4).Info("IPAddress not found, creating it.", "device", device)
		// IpAddress not yet created.
		err = machineScope.IPAMHelper.CreateIPAddressClaim(ctx, machineScope.ProxmoxMachine, device, poolNum, poolRef)
		if err != nil {
			return []ipamv1.IPAddress{}, errors.Wrapf(err, "unable to create Ip address claim for machine %s", machineScope.Name())
		}

		// send the machine to requeue so ipaddresses can be created
		return []ipamv1.IPAddress{}, nil
	}

	machineScope.Logger.V(4).Info("IPAddresses found, ", "ip", ipAddresses, "device", device)
	return ipAddresses, nil
}

func handleDevices(ctx context.Context, machineScope *scope.MachineScope, addresses map[string]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress) (bool, error) {
	// paranoidly handle callers handing us an empty map
	if addresses == nil {
		return false, errors.New("handleDevices called without a map")
	}

	networkSpec := ptr.Deref(machineScope.ProxmoxMachine.Spec.Network, infrav1.NetworkSpec{})

	poolsRef, err := GetInClusterIPPoolRefs(ctx, machineScope)
	if err != nil {
		return false, err
	}

	defaultIPv4 := networkSpec.DefaultNetworkSpec.ClusterPoolDeviceV4
	defaultIPv6 := networkSpec.DefaultNetworkSpec.ClusterPoolDeviceV6

	for _, net := range networkSpec.NetworkDevices {
		// TODO: Network Zones
		pools := []corev1.TypedLocalObjectReference{}

		// append default pools in front if they exist
		if defaultIPv4 != nil && *defaultIPv4 == *net.Name && poolsRef.IPv4 != nil {
			pools = append(pools, *poolsRef.IPv4)
		}
		if defaultIPv6 != nil && *defaultIPv6 == *net.Name && poolsRef.IPv6 != nil {
			pools = append(pools, *poolsRef.IPv6)
		}

		for i, ipPool := range slices.Concat(pools, net.InterfaceConfig.IPPoolRef) {
			ipAddresses, err := handleIPAddresses(ctx, machineScope, net.Name, i, ipPool)
			if err != nil {
				fmt.Println("handleDevices", "err", err, "ip", ipAddresses)
				return true, errors.Wrapf(err, "unable to handle IPAddress for device %+v, pool %s", net.Name, ipPool.Name)
			}

			// requeue machine if ipaddress need creation
			if len(ipAddresses) == 0 {
				return true, nil
			}

			poolMap := addresses[*net.Name]
			if poolMap == nil {
				poolMap = make(map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)
			}

			poolMap[ipPool] = ipAddresses
			addresses[*net.Name] = poolMap
		}
	}

	return false, nil
}

func isIPv4(ip string) bool {
	return netip.MustParseAddr(ip).Is4()
}
