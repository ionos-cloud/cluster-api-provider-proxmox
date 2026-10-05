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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/record"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	ipam "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcileIPAddresses(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition) != infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForStaticIPAllocationReason {
		// Machine is in the wrong state to reconcile, we only reconcile VMs Waiting for IP Address assignment
		return false, nil
	}

	machineScope.Logger.V(4).Info("reconciling IPAddresses.")
	pm := machineScope.ProxmoxMachine

	// TODO: This datastructure is less bad, but still bad
	netPoolAddresses := make(map[infrav1.NetName]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)

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
	writeIPAddressStatus(pm, netPoolAddresses)

	conditions.Set(pm, metav1.Condition{
		Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status: metav1.ConditionFalse,
		Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapDataReconciliationReason,
	})

	return true, nil
}

// writeIPAddressStatus republishes ProxmoxMachine.status.ipAddresses from the
// resolved per-net/pool addresses. Within each net the addresses are ordered by
// their pool-offset annotation and split into IPv4/IPv6 before being stored.
func writeIPAddressStatus(pm *infrav1.ProxmoxMachine, netPoolAddresses map[infrav1.NetName]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress) {
	for net, pools := range netPoolAddresses {
		addresses := slices.Concat(slices.Collect(maps.Values(pools))...)
		slices.SortFunc(addresses, func(a, b ipamv1.IPAddress) int {
			aOffset, _ := strconv.Atoi(a.GetAnnotations()[infrav1.ProxmoxPoolOffsetAnnotation])
			bOffset, _ := strconv.Atoi(b.GetAnnotations()[infrav1.ProxmoxPoolOffsetAnnotation])
			return aOffset - bOffset
		})
		ipSpec := infrav1.IPAddressesSpec{
			NetName: string(net),
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
}

// reconcileAddressRecovery republishes ProxmoxMachine IP/address status for an
// already-running machine (e.g. one restored from a backup) whose status was
// lost. It may recover CAPMOX ownership of a fully validated claim chain, but it
// never mutates an IPAddress or the VM. It is a no-op unless the VM is running
// and status.ipAddresses is empty, and only republishes when every expected
// IPAddressClaim resolves cleanly.
func reconcileAddressRecovery(ctx context.Context, machineScope *scope.MachineScope) error {
	pm := machineScope.ProxmoxMachine

	// Only act on already-running machines that are missing their IP status.
	if machineScope.VirtualMachine == nil || !machineScope.VirtualMachine.IsRunning() {
		return nil
	}
	if len(pm.Status.IPAddresses) > 0 || pm.Spec.Network == nil {
		return nil
	}

	machineScope.Logger.V(4).Info("attempting IP/address status recovery for running machine with empty status")

	netPoolAddresses := make(map[infrav1.NetName]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)
	incomplete, err := collectDeviceAddresses(ctx, machineScope, netPoolAddresses, resolveExistingIPAddress)
	if err != nil {
		return errors.Wrap(err, "resolving existing IPAddressClaims for status recovery")
	}
	// incomplete: at least one expected IPAddressClaim did not resolve cleanly.
	// len == 0: Spec.Network declares no NetworkDevices, so there is nothing to
	// republish (a populated map always carries the synthetic "default" net).
	// Either way, leave it to the regular provisioning path.
	if incomplete || len(netPoolAddresses) == 0 {
		machineScope.Logger.Info("address recovery skipped: no fully-resolved IPAddressClaims to republish")
		return nil
	}

	writeIPAddressStatus(pm, netPoolAddresses)

	addr, err := getClusterAPIMachineAddresses(machineScope)
	if err != nil {
		// status.ipAddresses was repopulated; CAPI machine addresses could not be
		// derived (e.g. no default network). Leave that to the regular path.
		machineScope.Logger.Info("recovered status.ipAddresses but could not derive machine addresses", "reason", err.Error())
		return nil
	}
	machineScope.SetAddresses(addr)

	machineScope.Logger.Info("recovered ProxmoxMachine IP and address status from existing IPAM claims")
	record.Eventf(pm, "RecoveredIPAddressStatus", "Republished IP/address status for running machine from existing IPAM claims")
	return nil
}

// resolveExistingIPAddress is the conservative resolver used by the
// status-recovery path. It may adopt a validated ownerless claim when a durable
// address already exists. It never creates a claim or mutates an IPAddress.
func resolveExistingIPAddress(ctx context.Context, machineScope *scope.MachineScope, ipClaimDef ipam.IPClaimDef) ([]ipamv1.IPAddress, error) {
	resolution, err := machineScope.IPAMHelper.ResolveIPAddressClaim(ctx, machineScope.ProxmoxMachine, ipClaimDef)
	if err != nil {
		return nil, err
	}
	if resolution.Status == ipam.ClaimAdoptable && resolution.Address != nil {
		resolution, err = adoptIPAddressClaim(ctx, machineScope, ipClaimDef)
		if err != nil {
			return nil, err
		}
	}
	if resolution.Status == ipam.ClaimResolved {
		return []ipamv1.IPAddress{*resolution.Address}, nil
	}
	if resolution.Status == ipam.ClaimConflict {
		// Recovery must not change the condition reason: it also controls the
		// provisioning steps that ReconcileVM executes immediately afterward.
		reportIPAddressClaimConflict(machineScope, resolution, ipClaimDef)
	}
	machineScope.Logger.V(4).Info("address recovery: IPAddressClaim not resolved",
		"claim", resolution.ClaimName, "status", resolution.Status, "device", ipClaimDef.Device)
	return []ipamv1.IPAddress{}, nil
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
		machineScope.ProxmoxMachine.Status.TaskRef = new(string(t.UPID))
		requeue = true
	}

	return requeue, nil
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
		rv = new(int32(metric))
	}

	if s, exists := annotations[infrav1.ProxmoxGatewayMetricAnnotation]; exists && len(s) > 0 {
		metric, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return nil, err
		}
		rv = new(int32(metric))
	}

	return rv, nil
}

func handleIPAddresses(ctx context.Context, machineScope *scope.MachineScope, ipClaimDef ipam.IPClaimDef) ([]ipamv1.IPAddress, error) {
	device := ipClaimDef.Device

	resolution, err := machineScope.IPAMHelper.ResolveIPAddressClaim(ctx, machineScope.ProxmoxMachine, ipClaimDef)
	if err != nil {
		return []ipamv1.IPAddress{}, err
	}
	if resolution.Status == ipam.ClaimAdoptable {
		resolution, err = adoptIPAddressClaim(ctx, machineScope, ipClaimDef)
		if err != nil {
			return []ipamv1.IPAddress{}, err
		}
	}

	switch resolution.Status {
	case ipam.ClaimMissing:
		if resolution.OrphanedAddress != nil {
			machineScope.Logger.V(4).Info("validated deterministic IPAddress for missing claim",
				"claim", resolution.ClaimName, "address", resolution.OrphanedAddress.Name, "device", device)
		}
		machineScope.Logger.V(4).Info("IPAddress not found, creating it.", "device", device)
		// IP address not yet created.
		created, err := machineScope.IPAMHelper.CreateIPAddressClaimIfMissing(ctx, machineScope.ProxmoxMachine, ipClaimDef)
		if err != nil {
			return []ipamv1.IPAddress{}, errors.Wrapf(err, "unable to create IP address claim for machine %s", machineScope.Name())
		}
		if created && resolution.OrphanedAddress != nil {
			record.Eventf(machineScope.ProxmoxMachine, "RecoveredIPAddressClaim",
				"Recreated IPAddressClaim %q for validated existing IPAddress %q", resolution.ClaimName, resolution.OrphanedAddress.Name)
			machineScope.Logger.Info("recreated IPAddressClaim for validated existing IPAddress",
				"claim", resolution.ClaimName, "address", resolution.OrphanedAddress.Name, "device", device)
		}

		// send the machine to requeue so ipaddresses can be created
		return []ipamv1.IPAddress{}, nil
	case ipam.ClaimPending:
		machineScope.Logger.V(4).Info("IPAddressClaim has no AddressRef yet.", "claim", resolution.ClaimName, "device", device)
		return []ipamv1.IPAddress{}, nil
	case ipam.ClaimResolved:
		machineScope.Logger.V(4).Info("IPAddresses found.", "ip", resolution.Address, "device", device)
		return []ipamv1.IPAddress{*resolution.Address}, nil
	case ipam.ClaimConflict:
		setIPAddressClaimConflict(machineScope, resolution, ipClaimDef)
		return []ipamv1.IPAddress{}, nil
	default:
		return []ipamv1.IPAddress{}, errors.Errorf("unknown IPAddressClaim resolution status %q", resolution.Status)
	}
}

func adoptIPAddressClaim(ctx context.Context, machineScope *scope.MachineScope, ipClaimDef ipam.IPClaimDef) (ipam.IPAddressClaimResolution, error) {
	resolution, adopted, err := machineScope.IPAMHelper.AdoptIPAddressClaim(ctx, machineScope.ProxmoxMachine, ipClaimDef)
	if err != nil {
		return resolution, errors.Wrapf(err, "unable to adopt IPAddressClaim %q", resolution.ClaimName)
	}
	if adopted {
		record.Eventf(machineScope.ProxmoxMachine, "AdoptedIPAddressClaim",
			"Adopted validated ownerless IPAddressClaim %q", resolution.ClaimName)
		machineScope.Logger.Info("adopted validated ownerless IPAddressClaim",
			"claim", resolution.ClaimName, "device", ipClaimDef.Device)
	}
	return resolution, nil
}

func setIPAddressClaimConflict(machineScope *scope.MachineScope, resolution ipam.IPAddressClaimResolution, ipClaimDef ipam.IPClaimDef) {
	message := reportIPAddressClaimConflict(machineScope, resolution, ipClaimDef)
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status:  metav1.ConditionFalse,
		Reason:  infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForStaticIPAllocationReason,
		Message: message,
	})
}

func reportIPAddressClaimConflict(machineScope *scope.MachineScope, resolution ipam.IPAddressClaimResolution, ipClaimDef ipam.IPClaimDef) string {
	message := fmt.Sprintf("Static IP claim %q is conflicting (%s): %s", resolution.ClaimName, resolution.ConflictReason, ipAddressClaimConflictDetail(machineScope, resolution, ipClaimDef))
	record.Warnf(machineScope.ProxmoxMachine, "IPAddressClaimConflict", "%s", message)
	machineScope.Logger.Info("IPAddressClaim conflict blocks static IP allocation", "claim", resolution.ClaimName, "reason", resolution.ConflictReason, "device", ipClaimDef.Device)
	return message
}

func ipAddressClaimConflictDetail(machineScope *scope.MachineScope, resolution ipam.IPAddressClaimResolution, ipClaimDef ipam.IPClaimDef) string {
	expectedCluster := machineScope.Cluster.Name
	switch resolution.ConflictReason {
	case ipam.ConflictClaimDeleting:
		return "expected an active IPAddressClaim, but deletion is in progress and its allocation may be released; wait for IPAM cleanup and inspect the allocation before recovery."
	case ipam.ConflictOwnerMismatch:
		return fmt.Sprintf("expected controller owner %s/%s with UID %q, actual ownerReferences are %v; inspect the restored IPAddressClaim before provisioning can continue.", infrav1.ProxmoxMachineKind, machineScope.Name(), machineScope.ProxmoxMachine.UID, resolution.Claim.OwnerReferences)
	case ipam.ConflictPoolMismatch:
		return fmt.Sprintf("expected poolRef %s/%s %q, actual poolRef is %s/%s %q; inspect the restored IPAddressClaim before provisioning can continue.", ptr.Deref(ipClaimDef.PoolRef.APIGroup, ""), ipClaimDef.PoolRef.Kind, ipClaimDef.PoolRef.Name, resolution.Claim.Spec.PoolRef.APIGroup, resolution.Claim.Spec.PoolRef.Kind, resolution.Claim.Spec.PoolRef.Name)
	case ipam.ConflictClaimCluster:
		return fmt.Sprintf("expected claim cluster label %q, actual value is %q; inspect the restored IPAddressClaim before provisioning can continue.", expectedCluster, resolution.Claim.Labels[clusterv1.ClusterNameLabel])
	case ipam.ConflictClaimAnnotations:
		return fmt.Sprintf("expected CAPMOX annotations %v, actual annotations are %v; inspect the restored IPAddressClaim before provisioning can continue.", ipClaimDef.Annotations, resolution.Claim.Annotations)
	case ipam.ConflictAddressMissing:
		return fmt.Sprintf("expected referenced IPAddress %q to exist; inspect the restored IPAM objects before provisioning can continue.", resolution.Claim.Status.AddressRef.Name)
	case ipam.ConflictAddressRef:
		return fmt.Sprintf("expected claim addressRef %q, actual value is %q; inspect the restored IPAddressClaim before provisioning can continue.", resolution.ClaimName, resolution.Claim.Status.AddressRef.Name)
	case ipam.ConflictAddressClaimRef:
		return fmt.Sprintf("expected IPAddress claimRef %q, actual value is %q; inspect the restored IPAddress before provisioning can continue.", resolution.ClaimName, conflictingAddress(resolution).Spec.ClaimRef.Name)
	case ipam.ConflictAddressPoolRef:
		actual := conflictingAddress(resolution).Spec.PoolRef
		return fmt.Sprintf("expected IPAddress poolRef %s/%s %q, actual poolRef is %s/%s %q; inspect the restored IPAddress before provisioning can continue.", ptr.Deref(ipClaimDef.PoolRef.APIGroup, ""), ipClaimDef.PoolRef.Kind, ipClaimDef.PoolRef.Name, actual.APIGroup, actual.Kind, actual.Name)
	case ipam.ConflictAddressCluster:
		return fmt.Sprintf("expected IPAddress cluster label %q, actual value is %q; inspect the restored IPAddress before provisioning can continue.", expectedCluster, conflictingAddress(resolution).Labels[clusterv1.ClusterNameLabel])
	case ipam.ConflictAddressOwner:
		return fmt.Sprintf("expected no conflicting IPAddress controller owner, actual ownerReferences are %v; inspect the restored IPAddress before provisioning can continue.", conflictingAddress(resolution).OwnerReferences)
	default:
		return "inspect the restored IPAddressClaim and IPAddress before provisioning can continue."
	}
}

func conflictingAddress(resolution ipam.IPAddressClaimResolution) *ipamv1.IPAddress {
	if resolution.ConflictingAddress != nil {
		return resolution.ConflictingAddress
	}
	if resolution.Address != nil {
		return resolution.Address
	}
	if resolution.OrphanedAddress != nil {
		return resolution.OrphanedAddress
	}
	return &ipamv1.IPAddress{}
}

// ipAddressResolver resolves the IPAddress(es) for a single device/pool claim
// definition. handleDevices uses the create-capable handleIPAddresses; the
// conservative status-recovery path uses resolveExistingIPAddress.
type ipAddressResolver func(ctx context.Context, machineScope *scope.MachineScope, ipClaimDef ipam.IPClaimDef) ([]ipamv1.IPAddress, error)

func handleDevices(ctx context.Context, machineScope *scope.MachineScope, addresses map[infrav1.NetName]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress) (bool, error) {
	return collectDeviceAddresses(ctx, machineScope, addresses, handleIPAddresses)
}

// collectDeviceAddresses walks the machine's network devices and pools, resolves
// each device/pool claim via the supplied resolver, and groups the resulting
// addresses into the per-net/pool map (including the synthetic "default" net).
// It returns requeue=true when any claim did not resolve to an address.
func collectDeviceAddresses(ctx context.Context, machineScope *scope.MachineScope, addresses map[infrav1.NetName]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress, resolve ipAddressResolver) (bool, error) {
	// paranoidly handle callers handing us an empty map
	if addresses == nil {
		return false, errors.New("collectDeviceAddresses called without a map")
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

			ipAddresses, err := resolve(ctx, machineScope, ipClaimDef)
			if err != nil {
				return true, errors.Wrapf(err, "unable to handle IPAddress for device %+v, pool %s", net.Name, ipPool.Name)
			}
			// fast track ip address generation with only one requeue
			if len(ipAddresses) == 0 {
				requeue = true
				continue
			}

			poolMap := addresses[net.Name]
			if poolMap == nil {
				poolMap = make(map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)
			}

			poolMap[ipPool] = append(poolMap[ipPool], ipAddresses...)
			addresses[net.Name] = poolMap

			// append default pool addresses to map
			if _, exists := defaultPoolMap[ipPool]; exists && (ptr.Deref(net.DefaultIPv4, false) || ptr.Deref(net.DefaultIPv6, false)) {
				defaultPoolMap[ipPool] = append(defaultPoolMap[ipPool], ipAddresses...)
			}
		}
		addresses["default"] = defaultPoolMap
	}

	return requeue, nil
}

func isIPv4(ip string) bool {
	return netip.MustParseAddr(ip).Is4()
}
