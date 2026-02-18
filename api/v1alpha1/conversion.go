/*
Copyright 2026 IONOS Cloud.

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

package v1alpha1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	capmoxv2 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// //
// v1alpha2 To v1alpha1 conversion functions
// //

func Convert_v1alpha2_NetworkSpec_To_v1alpha1_NetworkSpec(in *capmoxv2.NetworkSpec, out *NetworkSpec, s conversion.Scope) error {
	err := autoConvert_v1alpha2_NetworkSpec_To_v1alpha1_NetworkSpec(in, out, s)
	if err != nil {
		return err
	}

	err = convert_v1alpha2_DefaultNetwork_To_v1alpha1_DefaultNetwork(in, out, s)
	if err != nil {
		return err
	}

	err = convert_v1alpha2_AdditionalNetworkDevices_To_v1alpha1_AdditionalNetworkDevices(in, out, s)
	if err != nil {
		return err
	}

	return nil
}

// convert_v1alpha2_DefaultNetwork_To_v1alpha1_DefaultNetwork Default network device
func convert_v1alpha2_DefaultNetwork_To_v1alpha1_DefaultNetwork(in *capmoxv2.NetworkSpec, out *NetworkSpec, s conversion.Scope) error {
	i := getNetByName(in.NetworkDevices, DefaultNetworkDevice)

	if i >= 0 {
		out.Default = &NetworkDevice{}

		err := Convert_v1alpha2_NetworkDevice_To_v1alpha1_NetworkDevice(&in.NetworkDevices[i], out.Default, s)
		if err != nil {
			return err
		}
	}

	return nil
}

// convert_v1alpha2_AdditionalNetworkDevices_To_v1alpha1_AdditionalNetworkDevices Additional Network Devices
func convert_v1alpha2_AdditionalNetworkDevices_To_v1alpha1_AdditionalNetworkDevices(in *capmoxv2.NetworkSpec, out *NetworkSpec, s conversion.Scope) error {
	if in.NetworkDevices != nil {
		out.AdditionalDevices = make([]AdditionalNetworkDevice, 0, len(in.NetworkDevices))

		for i, net := range in.NetworkDevices {
			if net.Name != nil && *net.Name == DefaultNetworkDevice {
				// skip default device, already handled
				continue
			}

			nd := AdditionalNetworkDevice{}

			if net.Name != nil {
				nd.Name = *net.Name
			}

			err := Convert_v1alpha2_NetworkDevice_To_v1alpha1_NetworkDevice(&in.NetworkDevices[i], &nd.NetworkDevice, s)
			if err != nil {
				return err
			}

			err = Convert_v1alpha2_InterfaceConfig_To_v1alpha1_InterfaceConfig(&in.NetworkDevices[i].InterfaceConfig, &nd.InterfaceConfig, s)
			if err != nil {
				return err
			}

			out.AdditionalDevices = append(out.AdditionalDevices, nd)
		}

		if len(out.AdditionalDevices) == 0 {
			out.AdditionalDevices = nil
		}
	}

	return nil
}

func Convert_v1alpha2_NetworkDevice_To_v1alpha1_NetworkDevice(in *capmoxv2.NetworkDevice, out *NetworkDevice, s conversion.Scope) error {
	if in != nil {
		err := autoConvert_v1alpha2_NetworkDevice_To_v1alpha1_NetworkDevice(in, out, s)
		if err != nil {
			return err
		}

		if in.DNSServers != nil {
			out.DNSServers = in.DNSServers
		}

		if in.InterfaceConfig.IPPoolRef != nil {
			out.IPPoolConfig = IPPoolConfig{}

			// find IPv4 pool ref
			i := getIPPoolRefByIPFamily(in.InterfaceConfig.IPPoolRef, "inet")
			if i != -1 {
				out.IPPoolConfig.IPv4PoolRef = ptr.To(in.InterfaceConfig.IPPoolRef[i])
			}

			// find IPv6 pool ref
			j := getIPPoolRefByIPFamily(in.InterfaceConfig.IPPoolRef, "inet6")
			if j != -1 {
				out.IPPoolConfig.IPv6PoolRef = ptr.To(in.InterfaceConfig.IPPoolRef[j])
			}

			if i == -1 && j == -1 && len(in.InterfaceConfig.IPPoolRef) > 0 {
				out.IPPoolConfig.IPv4PoolRef = ptr.To(in.InterfaceConfig.IPPoolRef[0])
			}
		}

	} else {
		out = nil
	}

	return nil
}

func Convert_v1alpha2_InterfaceConfig_To_v1alpha1_InterfaceConfig(in *capmoxv2.InterfaceConfig, out *InterfaceConfig, s conversion.Scope) error {
	return autoConvert_v1alpha2_InterfaceConfig_To_v1alpha1_InterfaceConfig(in, out, s)
}

func Convert_v1alpha2_RouteSpec_To_v1alpha1_RouteSpec(in *capmoxv2.RouteSpec, out *RouteSpec, s conversion.Scope) error {
	if in == nil {
		return nil
	}

	err := autoConvert_v1alpha2_RouteSpec_To_v1alpha1_RouteSpec(in, out, s)
	if err != nil {
		return err
	}

	if in.Metric != nil {
		out.Metric = uint32(*in.Metric)
	}

	if in.Table != nil {
		out.Table = uint32(*in.Table)
	}
	return nil
}

func Convert_v1alpha2_RoutingPolicySpec_To_v1alpha1_RoutingPolicySpec(in *capmoxv2.RoutingPolicySpec, out *RoutingPolicySpec, s conversion.Scope) error {
	err := autoConvert_v1alpha2_RoutingPolicySpec_To_v1alpha1_RoutingPolicySpec(in, out, s)
	if err != nil {
		return err
	}

	if in.Priority != nil {
		out.Priority = uint32(*in.Priority)
	}
	return nil
}

func Convert_v1alpha1_ProxmoxClusterSpec_To_v1alpha2_ProxmoxClusterSpec(in *ProxmoxClusterSpec, out *capmoxv2.ProxmoxClusterSpec, s conversion.Scope) error {
	if err := autoConvert_v1alpha1_ProxmoxClusterSpec_To_v1alpha2_ProxmoxClusterSpec(in, out, s); err != nil {
		return err
	}
	// Manual conversion: *clusterv1.APIEndpoint → capmoxv2.APIEndpoint (value type)
	if in.ControlPlaneEndpoint != nil {
		out.ControlPlaneEndpoint = capmoxv2.APIEndpoint{
			Host: in.ControlPlaneEndpoint.Host,
			Port: in.ControlPlaneEndpoint.Port,
		}
	}
	return nil
}

func Convert_v1alpha2_ProxmoxClusterSpec_To_v1alpha1_ProxmoxClusterSpec(in *capmoxv2.ProxmoxClusterSpec, out *ProxmoxClusterSpec, s conversion.Scope) error {
	if err := autoConvert_v1alpha2_ProxmoxClusterSpec_To_v1alpha1_ProxmoxClusterSpec(in, out, s); err != nil {
		return err
	}
	// Manual conversion: capmoxv2.APIEndpoint (value type) → *clusterv1.APIEndpoint
	if in.ControlPlaneEndpoint.Host != "" || in.ControlPlaneEndpoint.Port != 0 {
		out.ControlPlaneEndpoint = &clusterv1.APIEndpoint{
			Host: in.ControlPlaneEndpoint.Host,
			Port: in.ControlPlaneEndpoint.Port,
		}
	}
	return nil
}

func Convert_v1alpha2_ProxmoxClusterCloneSpec_To_v1alpha1_ProxmoxClusterCloneSpec(in *capmoxv2.ProxmoxClusterCloneSpec, out *ProxmoxClusterCloneSpec, s conversion.Scope) error {
	err := autoConvert_v1alpha2_ProxmoxClusterCloneSpec_To_v1alpha1_ProxmoxClusterCloneSpec(in, out, s)
	if err != nil {
		return err
	}

	if in.ProxmoxClusterClassSpec != nil {
		out.ProxmoxMachineSpec = make(map[string]ProxmoxMachineSpec, len(in.ProxmoxClusterClassSpec))

		for _, pc := range in.ProxmoxClusterClassSpec {
			pms := ProxmoxMachineSpec{}
			err := Convert_v1alpha2_ProxmoxMachineSpec_To_v1alpha1_ProxmoxMachineSpec(&pc.ProxmoxMachineSpec, &pms, s)
			if err != nil {
				return err
			}

			out.ProxmoxMachineSpec[pc.MachineType] = pms
		}
	}

	return nil
}

func Convert_v1alpha1_ProxmoxClusterStatus_To_v1alpha2_ProxmoxClusterStatus(in *ProxmoxClusterStatus, out *capmoxv2.ProxmoxClusterStatus, s conversion.Scope) error {
	if err := autoConvert_v1alpha1_ProxmoxClusterStatus_To_v1alpha2_ProxmoxClusterStatus(in, out, s); err != nil {
		return err
	}
	// Manual conversion: Ready bool → Initialization.Provisioned *bool
	if in.Ready {
		out.Initialization.Provisioned = ptr.To(true)
	}
	// FailureReason and FailureMessage are dropped during up-conversion
	return nil
}

func Convert_v1alpha2_ProxmoxClusterStatus_To_v1alpha1_ProxmoxClusterStatus(in *capmoxv2.ProxmoxClusterStatus, out *ProxmoxClusterStatus, s conversion.Scope) error {
	// Accept WARNINGs: in.InClusterZoneRef, in.Initialization do not exist in peer-type
	if err := autoConvert_v1alpha2_ProxmoxClusterStatus_To_v1alpha1_ProxmoxClusterStatus(in, out, s); err != nil {
		return err
	}
	// Manual conversion: Initialization.Provisioned *bool → Ready bool
	out.Ready = ptr.Deref(in.Initialization.Provisioned, false)
	return nil
}

func Convert_v1alpha2_ProxmoxMachineStatus_To_v1alpha1_ProxmoxMachineStatus(in *capmoxv2.ProxmoxMachineStatus, out *ProxmoxMachineStatus, s conversion.Scope) error {
	err := autoConvert_v1alpha2_ProxmoxMachineStatus_To_v1alpha1_ProxmoxMachineStatus(in, out, s)
	if err != nil {
		return err
	}

	// Manual conversion: Initialization.Provisioned *bool → Ready bool
	out.Ready = ptr.Deref(in.Initialization.Provisioned, false)

	if in.VMStatus != nil {
		out.VMStatus = VirtualMachineState(ptr.Deref(in.VMStatus, capmoxv2.VirtualMachineStatePending))
	}

	if in.IPAddresses != nil {
		out.IPAddresses = make(map[string]IPAddress, len(in.IPAddresses))
		for _, v := range in.IPAddresses {
			ip := IPAddress{}
			if len(v.IPv4) > 0 {
				ip.IPV4 = v.IPv4[0]
			}
			if len(v.IPv6) > 0 {
				ip.IPV6 = v.IPv6[0]
			}

			out.IPAddresses[v.NetName] = ip
		}
	}

	if in.RetryAfter != nil {
		out.RetryAfter = ptr.Deref(in.RetryAfter, metav1.Time{})
	}

	return nil
}

func Convert_v1alpha2_NodeLocation_To_v1alpha1_NodeLocation(in *capmoxv2.NodeLocation, out *NodeLocation, s conversion.Scope) error {
	// accept the warning about unused fields here
	return autoConvert_v1alpha2_NodeLocation_To_v1alpha1_NodeLocation(in, out, s)
}

// //
// v1alpha1 To v1alpha2 conversion functions
// //

func Convert_v1alpha1_NetworkSpec_To_v1alpha2_NetworkSpec(in *NetworkSpec, out *capmoxv2.NetworkSpec, s conversion.Scope) error {
	err := autoConvert_v1alpha1_NetworkSpec_To_v1alpha2_NetworkSpec(in, out, s)
	if err != nil {
		return err
	}

	if in.Default != nil {
		net0 := capmoxv2.NetworkDevice{
			Name:        ptr.To(DefaultNetworkDevice),
			DefaultIPv4: ptr.To(true),
			DefaultIPv6: ptr.To(true),
			InterfaceConfig: capmoxv2.InterfaceConfig{
				IPPoolRef: make([]corev1.TypedLocalObjectReference, 0),
			},
		}

		err = Convert_v1alpha1_NetworkDevice_To_v1alpha2_NetworkDevice(in.Default, &net0, s)
		if err != nil {
			return err
		}

		out.NetworkDevices = append(out.NetworkDevices, net0)
	}

	// additional devices
	for _, device := range in.AdditionalDevices {
		net := capmoxv2.NetworkDevice{
			Name: ptr.To(device.Name),
			InterfaceConfig: capmoxv2.InterfaceConfig{
				IPPoolRef: make([]corev1.TypedLocalObjectReference, 0),
			},
		}

		err = Convert_v1alpha1_NetworkDevice_To_v1alpha2_NetworkDevice(&device.NetworkDevice, &net, s)
		if err != nil {
			return err
		}

		err = Convert_v1alpha1_InterfaceConfig_To_v1alpha2_InterfaceConfig(&device.InterfaceConfig, &net.InterfaceConfig, s)
		if err != nil {
			return err
		}

		out.NetworkDevices = append(out.NetworkDevices, net)
	}

	return nil
}

func Convert_v1alpha1_NetworkDevice_To_v1alpha2_NetworkDevice(in *NetworkDevice, out *capmoxv2.NetworkDevice, s conversion.Scope) error {
	err := autoConvert_v1alpha1_NetworkDevice_To_v1alpha2_NetworkDevice(in, out, s)
	if err != nil {
		return err
	}
	if in.DNSServers != nil {
		out.DNSServers = in.DNSServers
	}
	if in.IPPoolConfig.IPv4PoolRef != nil {
		out.InterfaceConfig.IPPoolRef = append(out.InterfaceConfig.IPPoolRef, *in.IPv4PoolRef)
	}
	if in.IPPoolConfig.IPv6PoolRef != nil {
		out.InterfaceConfig.IPPoolRef = append(out.InterfaceConfig.IPPoolRef, *in.IPv6PoolRef)
	}

	return nil
}

func Convert_v1alpha1_RouteSpec_To_v1alpha2_RouteSpec(in *RouteSpec, out *capmoxv2.RouteSpec, s conversion.Scope) error {
	if in == nil {
		return nil
	}

	err := autoConvert_v1alpha1_RouteSpec_To_v1alpha2_RouteSpec(in, out, s)
	if err != nil {
		return err
	}

	if in.Metric > 0 {
		out.Metric = ptr.To(int32(in.Metric))
	}

	if in.Table > 0 {
		out.Table = ptr.To(int32(in.Table))
	}
	return nil
}

func Convert_v1alpha1_RoutingPolicySpec_To_v1alpha2_RoutingPolicySpec(in *RoutingPolicySpec, out *capmoxv2.RoutingPolicySpec, s conversion.Scope) error {
	err := autoConvert_v1alpha1_RoutingPolicySpec_To_v1alpha2_RoutingPolicySpec(in, out, s)
	if err != nil {
		return err
	}
	if in.Priority > 0 {
		out.Priority = ptr.To(int64(in.Priority))
	}
	return nil
}

func Convert_v1alpha1_ProxmoxClusterCloneSpec_To_v1alpha2_ProxmoxClusterCloneSpec(in *ProxmoxClusterCloneSpec, out *capmoxv2.ProxmoxClusterCloneSpec, s conversion.Scope) error {
	err := autoConvert_v1alpha1_ProxmoxClusterCloneSpec_To_v1alpha2_ProxmoxClusterCloneSpec(in, out, s)
	if err != nil {
		return err
	}

	if in.ProxmoxMachineSpec != nil {
		out.ProxmoxClusterClassSpec = make([]capmoxv2.ProxmoxClusterClassSpec, 0, len(in.ProxmoxMachineSpec))

		for k, v := range in.ProxmoxMachineSpec {
			pms := capmoxv2.ProxmoxMachineSpec{}
			err := Convert_v1alpha1_ProxmoxMachineSpec_To_v1alpha2_ProxmoxMachineSpec(&v, &pms, s)
			if err != nil {
				return err
			}

			pc := capmoxv2.ProxmoxClusterClassSpec{
				MachineType:        k,
				ProxmoxMachineSpec: pms,
			}

			out.ProxmoxClusterClassSpec = append(out.ProxmoxClusterClassSpec, pc)
		}
	}

	return nil
}

func Convert_v1alpha1_ProxmoxMachineStatus_To_v1alpha2_ProxmoxMachineStatus(in *ProxmoxMachineStatus, out *capmoxv2.ProxmoxMachineStatus, s conversion.Scope) error {
	err := autoConvert_v1alpha1_ProxmoxMachineStatus_To_v1alpha2_ProxmoxMachineStatus(in, out, s)
	if err != nil {
		return err
	}

	// Manual conversion: Ready bool → Initialization.Provisioned *bool
	if in.Ready {
		out.Initialization.Provisioned = ptr.To(true)
	}

	if in.VMStatus != "" {
		vmState := capmoxv2.VirtualMachineState(in.VMStatus)
		out.VMStatus = &vmState
	}

	if in.IPAddresses != nil {
		out.IPAddresses = make([]capmoxv2.IPAddressesSpec, 0, len(in.IPAddresses))
		for netName, ipAddr := range in.IPAddresses {
			ipSpec := capmoxv2.IPAddressesSpec{
				NetName: netName,
			}
			if ipAddr.IPV4 != "" {
				ipSpec.IPv4 = []string{ipAddr.IPV4}
			}
			if ipAddr.IPV6 != "" {
				ipSpec.IPv6 = []string{ipAddr.IPV6}
			}
			out.IPAddresses = append(out.IPAddresses, ipSpec)
		}
	}

	if !in.RetryAfter.IsZero() {
		t := metav1.Time{Time: in.RetryAfter.Time}
		out.RetryAfter = &t
	}

	return nil
}

func Convert_v1alpha1_ProxmoxClusterTemplateResource_To_v1alpha2_ProxmoxClusterTemplateResource(in *ProxmoxClusterTemplateResource, out *capmoxv2.ProxmoxClusterTemplateResource, s conversion.Scope) error {
	out.ObjectMeta = clusterv1beta2.ObjectMeta{
		Labels:      in.ObjectMeta.Labels,
		Annotations: in.ObjectMeta.Annotations,
	}
	return Convert_v1alpha1_ProxmoxClusterSpec_To_v1alpha2_ProxmoxClusterSpec(&in.Spec, &out.Spec, s)
}

func Convert_v1alpha2_ProxmoxClusterTemplateResource_To_v1alpha1_ProxmoxClusterTemplateResource(in *capmoxv2.ProxmoxClusterTemplateResource, out *ProxmoxClusterTemplateResource, s conversion.Scope) error {
	out.ObjectMeta = clusterv1.ObjectMeta{
		Labels:      in.ObjectMeta.Labels,
		Annotations: in.ObjectMeta.Annotations,
	}
	return Convert_v1alpha2_ProxmoxClusterSpec_To_v1alpha1_ProxmoxClusterSpec(&in.Spec, &out.Spec, s)
}

func Convert_v1alpha1_ProxmoxMachineTemplateResource_To_v1alpha2_ProxmoxMachineTemplateResource(in *ProxmoxMachineTemplateResource, out *capmoxv2.ProxmoxMachineTemplateResource, s conversion.Scope) error {
	out.ObjectMeta = clusterv1beta2.ObjectMeta{
		Labels:      in.ObjectMeta.Labels,
		Annotations: in.ObjectMeta.Annotations,
	}
	return Convert_v1alpha1_ProxmoxMachineSpec_To_v1alpha2_ProxmoxMachineSpec(&in.Spec, &out.Spec, s)
}

func Convert_v1alpha2_ProxmoxMachineTemplateResource_To_v1alpha1_ProxmoxMachineTemplateResource(in *capmoxv2.ProxmoxMachineTemplateResource, out *ProxmoxMachineTemplateResource, s conversion.Scope) error {
	out.ObjectMeta = clusterv1.ObjectMeta{
		Labels:      in.ObjectMeta.Labels,
		Annotations: in.ObjectMeta.Annotations,
	}
	return Convert_v1alpha2_ProxmoxMachineSpec_To_v1alpha1_ProxmoxMachineSpec(&in.Spec, &out.Spec, s)
}

// compileErrorOnMissingConversion is a stub to satisfy dead-code references
// in conversion-gen's autoConvert_ functions for ObjectMeta type mismatches.
// These autoConvert_ functions are never called because we provide manual
// Convert_ wrappers that handle the conversion directly.
func compileErrorOnMissingConversion() {}

// //
// helpers
// //

func Convert_v1alpha2_NetName_To_string(in *capmoxv2.NetName, out *string, s conversion.Scope) error {
	*out = ptr.Deref(*in, "")
	return nil
}

func Convert_string_To_v1alpha2_NetName(in *string, out *capmoxv2.NetName, s conversion.Scope) error {
	*out = in
	return nil
}

func Convert_Slice_v1alpha2_NetName_To_Slice_string(in *[]capmoxv2.NetName, out *[]string, s conversion.Scope) error {
	if in != nil {
		*out = make([]string, 0, len(*in))
		for _, n := range *in {
			var str string
			err := Convert_v1alpha2_NetName_To_string(&n, &str, s)
			if err != nil {
				return err
			}

			*out = append(*out, str)
		}
	}

	return nil
}

func Convert_Slice_string_To_Slice_v1alpha2_NetName(in *[]string, out *[]capmoxv2.NetName, s conversion.Scope) error {
	if in != nil {
		*out = make([]capmoxv2.NetName, 0, len(*in))
		for _, str := range *in {
			var n capmoxv2.NetName
			err := Convert_string_To_v1alpha2_NetName(&str, &n, s)
			if err != nil {
				return err
			}

			*out = append(*out, n)
		}
	}

	return nil
}

func getNetByName(nets []capmoxv2.NetworkDevice, name string) int {
	for i, net := range nets {
		if net.Name != nil && *net.Name == name {
			return i
		}
	}
	return -1
}

func getIPPoolRefByIPFamily(poolRefs []corev1.TypedLocalObjectReference, ipFamily string) int {
	for i, ref := range poolRefs {
		if strings.Contains(ref.Name, ipFamily) {
			return i
		}
	}

	return -1
}

func Convert_string_To_Pointer_string(in string, hasRestored bool, restoredIn *string, out **string) {
	// If the value is "", convert to *"" only if the value was *"" before (we know it was intentionally set to "").
	// In all the other cases we do not know if the value was intentionally set to "", so convert to nil.
	if in == "" {
		if hasRestored && restoredIn != nil && *restoredIn == "" {
			*out = ptr.To("")
			return
		}
		*out = nil
		return
	}

	// Otherwise, if the value is not "", convert to *value.
	*out = ptr.To(in)
}

func Convert_string_To_NetName(in string, hasRestored bool, restoredIn capmoxv2.NetName, out *capmoxv2.NetName) {
	if in == "" {
		if hasRestored && restoredIn != nil && *restoredIn == "" {
			*out = ptr.To("")
			return
		}
		*out = nil
		return
	}

	// Otherwise, if the value is not "", convert to *value.
	*out = ptr.To(in)
}

// Convert_v1beta1_Condition_To_v1_Condition is the conversion stub required by conversion-gen
// to convert clusterv1.Condition (v1beta1) to metav1.Condition (v1).
func Convert_v1beta1_Condition_To_v1_Condition(in *clusterv1.Condition, out *metav1.Condition, s conversion.Scope) error {
	out.Type = string(in.Type)
	out.Status = metav1.ConditionStatus(in.Status)
	out.LastTransitionTime = in.LastTransitionTime
	out.Reason = in.Reason
	out.Message = in.Message
	return nil
}

// Convert_v1_Condition_To_v1beta1_Condition is the conversion stub required by conversion-gen
// to convert metav1.Condition (v1) to clusterv1.Condition (v1beta1).
func Convert_v1_Condition_To_v1beta1_Condition(in *metav1.Condition, out *clusterv1.Condition, s conversion.Scope) error {
	out.Type = clusterv1.ConditionType(in.Type)
	out.Status = corev1.ConditionStatus(in.Status)
	out.LastTransitionTime = in.LastTransitionTime
	out.Reason = in.Reason
	out.Message = in.Message
	return nil
}

func getNetDeviceByName(nets []AdditionalNetworkDevice, name string) *AdditionalNetworkDevice {
	for _, net := range nets {
		if net.Name == name {
			return &net
		}
	}
	return nil
}

// normalizeProxmoxMachineSpec normalizes a ProxmoxMachineSpec after conversion.
func normalizeProxmoxMachineSpec(spec *capmoxv2.ProxmoxMachineSpec) {
	if spec.Network == nil {
		return
	}

	// Normalize VRF interfaces and routing
	for i := range spec.Network.VRFs {
		vrf := &spec.Network.VRFs[i]

		// Empty slice created by auto-conversion
		if len(vrf.Interfaces) == 0 {
			vrf.Interfaces = nil
		}

		// Normalize routing policy empty strings
		for j := range vrf.Routing.RoutingPolicy {
			policy := &vrf.Routing.RoutingPolicy[j]
			if policy.To != nil && *policy.To == "" {
				policy.To = nil
			}
			if policy.From != nil && *policy.From == "" {
				policy.From = nil
			}
		}

		// Normalize route empty strings
		for j := range vrf.Routing.Routes {
			route := &vrf.Routing.Routes[j]
			if route.To != nil && *route.To == "" {
				route.To = nil
			}
			if route.Via != nil && *route.Via == "" {
				route.Via = nil
			}
		}
	}
}
