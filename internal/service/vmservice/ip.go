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
	"maps"
	"net/netip"
	"slices"
	"strconv"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"
	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	ipam "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcileIPAddresses(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.VMProvisionedCondition) != infrav1.WaitingForStaticIPAllocationReason {
		// Machine is in the wrong state to reconcile, we only reconcile VMs Waiting for IP Address assignment
		return false, nil
	}

	machineScope.Logger.V(4).Info("reconciling IPAddresses.")
	pm := machineScope.ProxmoxMachine

	// TODO: This datastructure is less bad, but still bad
	netPoolAddresses := make(map[string]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)

	if pm.Spec.Network != nil {
		if requeue, err = handleDevices(ctx, machineScope, netPoolAddresses); err != nil || requeue {
			if err == nil {
				return true, errors.Wrap(err, "requeuing network reconcillation")
			}
			return true, errors.Wrap(err, "unable to handle network devices")
		}
	}

	// TODO: move to own state machine stage. Doesn't belong here, really
	defaultDevicePools := netPoolAddresses["default"]
	for _, ipAddresses := range defaultDevicePools {
		// Todo: This is not necessarily the default IP.
		_, err := setVMIPAddressTag(ctx, machineScope, ipAddresses[0])
		if err != nil {
			return false, err
		}
	}

	machineScope.Logger.V(4).Info("updating the ProxmoxMachine's IP addresses.")
	for net, pools := range netPoolAddresses {
		addresses := slices.Concat(slices.Collect(maps.Values(pools))...)
		slices.SortFunc(addresses, func(a, b ipamv1.IPAddress) int {
			aOffset, _ := strconv.Atoi(a.GetAnnotations()[infrav1.ProxmoxPoolOffsetAnnotation])
			bOffset, _ := strconv.Atoi(b.GetAnnotations()[infrav1.ProxmoxPoolOffsetAnnotation])
			return aOffset - bOffset
		})
		ipSpec := infrav1.IPAddressesSpec{
			NetName: net,
		}
		for _, address := range addresses {
			if isIPv4(address.Spec.Address) {
				ipSpec.IPv4 = append(ipSpec.IPv4, address.Spec.Address)
			} else {
				ipSpec.IPv6 = append(ipSpec.IPv6, address.Spec.Address)
			}
		}
		pm.SetIPAddresses(ipSpec)
	}

	conditions.MarkFalse(pm, infrav1.VMProvisionedCondition, infrav1.WaitingForBootstrapDataReconcilationReason, clusterv1.ConditionSeverityInfo, "")

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
			return false, errors.Wrapf(err, "unable to add IP tag to VirtualMachine %s", machineScope.Name())
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

func handleIPAddresses(ctx context.Context, machineScope *scope.MachineScope, ipClaimDef ipam.IPClaimDef) ([]ipamv1.IPAddress, error) {
	device := ptr.Deref(ipClaimDef.Device, infrav1.DefaultNetworkDevice)

	ipAddresses, err := findIPAddress(ctx, ipClaimDef.PoolRef, machineScope)
	if err != nil {
		// Technically this error cannot occur as fieldselectors just return empty lists
		if !apierrors.IsNotFound(err) {
			return []ipamv1.IPAddress{}, err
		}
	}

	index := slices.IndexFunc(ipAddresses, func(ip ipamv1.IPAddress) bool {
		return ip.GetAnnotations()[infrav1.ProxmoxPoolOffsetAnnotation] == ipClaimDef.Annotations[infrav1.ProxmoxPoolOffsetAnnotation]
	})

	if index < 0 {
		machineScope.Logger.V(4).Info("IPAddress not found, creating it.", "device", device)
		// IP address not yet created.
		err = machineScope.IPAMHelper.CreateIPAddressClaim(ctx, machineScope.ProxmoxMachine, ipClaimDef)
		if err != nil {
			return []ipamv1.IPAddress{}, errors.Wrapf(err, "unable to create IP address claim for machine %s", machineScope.Name())
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

	defaultPoolMap := make(map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)

	requeue := false
	for _, net := range networkSpec.NetworkDevices {
		pools := []corev1.TypedLocalObjectReference{}

		// append default pools in front if they exist.
		if ptr.Deref(net.DefaultIPv4, false) && poolsRef.IPv4 != nil {
			pools = append(pools, *poolsRef.IPv4)
			defaultPoolMap[*poolsRef.IPv4] = []ipamv1.IPAddress{}
		}
		if ptr.Deref(net.DefaultIPv6, false) && poolsRef.IPv6 != nil {
			pools = append(pools, *poolsRef.IPv6)
			defaultPoolMap[*poolsRef.IPv6] = []ipamv1.IPAddress{}
		}

		for i, ipPool := range slices.Concat(pools, net.InterfaceConfig.IPPoolRef) {
			ipClaimDef := ipam.IPClaimDef{
				PoolRef: ipPool,
				Device:  net.Name,
				Annotations: map[string]string{
					infrav1.ProxmoxPoolOffsetAnnotation: fmt.Sprintf("%d", i),
				},
			}
			// TODO: I hate this default pool logic
			if ptr.Deref(net.DefaultIPv4, false) &&
				ipPool == ptr.Deref(poolsRef.IPv4, corev1.TypedLocalObjectReference{}) ||
				ptr.Deref(net.DefaultIPv6, false) &&
					ipPool == ptr.Deref(poolsRef.IPv6, corev1.TypedLocalObjectReference{}) {
				ipClaimDef.Annotations[infrav1.ProxmoxDefaultGatewayAnnotation] = "true"
			}

			ipAddresses, err := handleIPAddresses(ctx, machineScope, ipClaimDef)
			if err != nil {
				return true, errors.Wrapf(err, "unable to handle IPAddress for device %+v, pool %s", net.Name, ipPool.Name)
			}
			// fast track ip address generation with only one requeue
			if len(ipAddresses) == 0 {
				requeue = true
				continue
			}

			poolMap := addresses[*net.Name]
			if poolMap == nil {
				poolMap = make(map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)
			}

			poolMap[ipPool] = ipAddresses
			addresses[*net.Name] = poolMap

			// append default pool addresses to map
			if _, exists := defaultPoolMap[ipPool]; exists && ptr.Deref(net.DefaultIPv4, false) || ptr.Deref(net.DefaultIPv6, false) {
				defaultPoolMap[ipPool] = ipAddresses
			}
		}
		addresses["default"] = defaultPoolMap
	}

	return requeue, nil
}

func isIPv4(ip string) bool {
	return netip.MustParseAddr(ip).Is4()
}
