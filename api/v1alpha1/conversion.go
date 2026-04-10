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
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// //
// v1alpha2 To v1alpha1 conversion functions
// //

func Convert_v1alpha2_NetworkSpec_To_v1alpha1_NetworkSpec(in *v1alpha2.NetworkSpec, out *NetworkSpec, s conversion.Scope) error {
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
func convert_v1alpha2_DefaultNetwork_To_v1alpha1_DefaultNetwork(in *v1alpha2.NetworkSpec, out *NetworkSpec, s conversion.Scope) error {
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
func convert_v1alpha2_AdditionalNetworkDevices_To_v1alpha1_AdditionalNetworkDevices(in *v1alpha2.NetworkSpec, out *NetworkSpec, s conversion.Scope) error {
	if in.NetworkDevices != nil {
		out.AdditionalDevices = make([]AdditionalNetworkDevice, 0, len(in.NetworkDevices))

		for i, net := range in.NetworkDevices {
			if string(net.Name) == DefaultNetworkDevice {
				// skip default device, already handled
				continue
			}

			nd := AdditionalNetworkDevice{}
			nd.Name = string(net.Name)

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

func Convert_v1alpha2_NetworkDevice_To_v1alpha1_NetworkDevice(in *v1alpha2.NetworkDevice, out *NetworkDevice, s conversion.Scope) error {
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

func Convert_v1alpha2_InterfaceConfig_To_v1alpha1_InterfaceConfig(in *v1alpha2.InterfaceConfig, out *InterfaceConfig, s conversion.Scope) error {
	return autoConvert_v1alpha2_InterfaceConfig_To_v1alpha1_InterfaceConfig(in, out, s)
}

func Convert_v1alpha2_RouteSpec_To_v1alpha1_RouteSpec(in *v1alpha2.RouteSpec, out *RouteSpec, s conversion.Scope) error {
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

func Convert_v1alpha2_RoutingPolicySpec_To_v1alpha1_RoutingPolicySpec(in *v1alpha2.RoutingPolicySpec, out *RoutingPolicySpec, s conversion.Scope) error {
	err := autoConvert_v1alpha2_RoutingPolicySpec_To_v1alpha1_RoutingPolicySpec(in, out, s)
	if err != nil {
		return err
	}

	if in.Priority != nil {
		out.Priority = uint32(*in.Priority)
	}
	return nil
}

func Convert_v1alpha2_ProxmoxClusterSpec_To_v1alpha1_ProxmoxClusterSpec(in *v1alpha2.ProxmoxClusterSpec, out *ProxmoxClusterSpec, s conversion.Scope) error {
	if err := autoConvert_v1alpha2_ProxmoxClusterSpec_To_v1alpha1_ProxmoxClusterSpec(in, out, s); err != nil {
		return err
	}

	// Manual conversion: v1alpha2.APIEndpoint (value type) → *clusterv1beta1.APIEndpoint
	if !in.ControlPlaneEndpoint.IsZero() {
		out.ControlPlaneEndpoint = &clusterv1beta1.APIEndpoint{
			Host: in.ControlPlaneEndpoint.Host,
			Port: in.ControlPlaneEndpoint.Port,
		}
	}

	return nil
}

func Convert_v1alpha1_ProxmoxClusterStatus_To_v1alpha2_ProxmoxClusterStatus(in *ProxmoxClusterStatus, out *v1alpha2.ProxmoxClusterStatus, s conversion.Scope) error {
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

func Convert_v1alpha2_ProxmoxClusterStatus_To_v1alpha1_ProxmoxClusterStatus(in *v1alpha2.ProxmoxClusterStatus, out *ProxmoxClusterStatus, s conversion.Scope) error {
	// Accept WARNING: in.InClusterZoneRef does not exist in peer-type
	if err := autoConvert_v1alpha2_ProxmoxClusterStatus_To_v1alpha1_ProxmoxClusterStatus(in, out, s); err != nil {
		return err
	}

	// Manual conversion: Initialization.Provisioned *bool → Ready bool
	out.Ready = ptr.Deref(in.Initialization.Provisioned, false)

	return nil
}

func Convert_v1alpha2_ProxmoxMachineStatus_To_v1alpha1_ProxmoxMachineStatus(in *v1alpha2.ProxmoxMachineStatus, out *ProxmoxMachineStatus, s conversion.Scope) error {
	err := autoConvert_v1alpha2_ProxmoxMachineStatus_To_v1alpha1_ProxmoxMachineStatus(in, out, s)
	if err != nil {
		return err
	}

	// Manual conversion: Initialization.Provisioned *bool → Ready bool
	out.Ready = ptr.Deref(in.Initialization.Provisioned, false)

	if in.VMStatus != nil {
		out.VMStatus = VirtualMachineState(ptr.Deref(in.VMStatus, v1alpha2.VirtualMachineStatePending))
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

			out.IPAddresses[string(v.NetName)] = ip
		}
	}

	if in.RetryAfter != nil {
		out.RetryAfter = ptr.Deref(in.RetryAfter, metav1.Time{})
	}

	return nil
}

func Convert_v1alpha2_NodeLocation_To_v1alpha1_NodeLocation(in *v1alpha2.NodeLocation, out *NodeLocation, s conversion.Scope) error {
	// accept the warning about unused fields here
	return autoConvert_v1alpha2_NodeLocation_To_v1alpha1_NodeLocation(in, out, s)
}

func Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in *clusterv1beta2.ObjectMeta, out *clusterv1beta1.ObjectMeta, s conversion.Scope) error {
	if err := clusterv1beta1.Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in, out, s); err != nil {
		return err
	}

	return nil
}

// //
// v1alpha1 To v1alpha2 conversion functions
// //

func Convert_v1alpha1_ProxmoxClusterSpec_To_v1alpha2_ProxmoxClusterSpec(in *ProxmoxClusterSpec, out *v1alpha2.ProxmoxClusterSpec, s conversion.Scope) error {
	if in == nil {
		return nil
	}

	// Manual conversion: *clusterv1beta1.APIEndpoint → v1alpha2.APIEndpoint (value type)
	if in.ControlPlaneEndpoint != nil {
		out.ControlPlaneEndpoint = v1alpha2.APIEndpoint{
			Host: in.ControlPlaneEndpoint.Host,
			Port: in.ControlPlaneEndpoint.Port,
		}
	}

	out.ExternalManagedControlPlane = &in.ExternalManagedControlPlane
	out.AllowedNodes = in.AllowedNodes

	if in.SchedulerHints != nil {
		out.SchedulerHints = &v1alpha2.SchedulerHints{}
		err := autoConvert_v1alpha1_SchedulerHints_To_v1alpha2_SchedulerHints(in.SchedulerHints, out.SchedulerHints, s)

		if err != nil {
			return err
		}
	}

	if in.IPv4Config != nil {
		out.IPv4Config = &v1alpha2.IPConfigSpec{}
		err := autoConvert_v1alpha1_IPConfigSpec_To_v1alpha2_IPConfigSpec(in.IPv4Config, out.IPv4Config, s)

		if err != nil {
			return err
		}
	}

	if in.IPv6Config != nil {
		out.IPv6Config = &v1alpha2.IPConfigSpec{}
		err := autoConvert_v1alpha1_IPConfigSpec_To_v1alpha2_IPConfigSpec(in.IPv6Config, out.IPv6Config, s)

		if err != nil {
			return err
		}
	}

	out.DNSServers = in.DNSServers
	out.ZoneConfigs = []v1alpha2.ZoneConfigSpec{}
	out.CredentialsRef = in.CredentialsRef

	return nil
}

func Convert_v1alpha1_NetworkSpec_To_v1alpha2_NetworkSpec(in *NetworkSpec, out *v1alpha2.NetworkSpec, s conversion.Scope) error {
	err := autoConvert_v1alpha1_NetworkSpec_To_v1alpha2_NetworkSpec(in, out, s)
	if err != nil {
		return err
	}

	if in.Default != nil {
		net0 := v1alpha2.NetworkDevice{
			Name:        v1alpha2.NetName(DefaultNetworkDevice),
			DefaultIPv4: ptr.To(true),
			DefaultIPv6: ptr.To(true),
			InterfaceConfig: v1alpha2.InterfaceConfig{
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
		net := v1alpha2.NetworkDevice{
			Name: v1alpha2.NetName(device.Name),
			InterfaceConfig: v1alpha2.InterfaceConfig{
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

func Convert_v1alpha1_NetworkDevice_To_v1alpha2_NetworkDevice(in *NetworkDevice, out *v1alpha2.NetworkDevice, s conversion.Scope) error {
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

func Convert_v1alpha1_RouteSpec_To_v1alpha2_RouteSpec(in *RouteSpec, out *v1alpha2.RouteSpec, s conversion.Scope) error {
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

func Convert_v1alpha1_RoutingPolicySpec_To_v1alpha2_RoutingPolicySpec(in *RoutingPolicySpec, out *v1alpha2.RoutingPolicySpec, s conversion.Scope) error {
	err := autoConvert_v1alpha1_RoutingPolicySpec_To_v1alpha2_RoutingPolicySpec(in, out, s)
	if err != nil {
		return err
	}
	if in.Priority > 0 {
		out.Priority = ptr.To(int64(in.Priority))
	}
	return nil
}

func Convert_v1alpha1_ProxmoxMachineStatus_To_v1alpha2_ProxmoxMachineStatus(in *ProxmoxMachineStatus, out *v1alpha2.ProxmoxMachineStatus, s conversion.Scope) error {
	err := autoConvert_v1alpha1_ProxmoxMachineStatus_To_v1alpha2_ProxmoxMachineStatus(in, out, s)
	if err != nil {
		return err
	}

	// Manual conversion: Ready bool → Initialization.Provisioned *bool
	if in.Ready {
		out.Initialization.Provisioned = ptr.To(true)
	}

	if in.VMStatus != "" {
		vmState := v1alpha2.VirtualMachineState(in.VMStatus)
		out.VMStatus = &vmState
	}

	if in.IPAddresses != nil {
		out.IPAddresses = make([]v1alpha2.IPAddressesSpec, 0, len(in.IPAddresses))
		for netName, ipAddr := range in.IPAddresses {
			ipSpec := v1alpha2.IPAddressesSpec{
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

func Convert_v1alpha1_ProxmoxClusterTemplateResource_To_v1alpha2_ProxmoxClusterTemplateResource(in *ProxmoxClusterTemplateResource, out *v1alpha2.ProxmoxClusterTemplateResource, s conversion.Scope) error {
	out.ObjectMeta = clusterv1beta2.ObjectMeta{
		Labels:      in.ObjectMeta.Labels,
		Annotations: in.ObjectMeta.Annotations,
	}

	return Convert_v1alpha1_ProxmoxClusterSpec_To_v1alpha2_ProxmoxClusterSpec(&in.Spec, &out.Spec, s)
}

func Convert_v1alpha2_ProxmoxClusterTemplateResource_To_v1alpha1_ProxmoxClusterTemplateResource(in *v1alpha2.ProxmoxClusterTemplateResource, out *ProxmoxClusterTemplateResource, s conversion.Scope) error {
	out.ObjectMeta = clusterv1beta1.ObjectMeta{
		Labels:      in.ObjectMeta.Labels,
		Annotations: in.ObjectMeta.Annotations,
	}
	return Convert_v1alpha2_ProxmoxClusterSpec_To_v1alpha1_ProxmoxClusterSpec(&in.Spec, &out.Spec, s)
}

func Convert_v1alpha1_ProxmoxMachineTemplateResource_To_v1alpha2_ProxmoxMachineTemplateResource(in *ProxmoxMachineTemplateResource, out *v1alpha2.ProxmoxMachineTemplateResource, s conversion.Scope) error {
	out.ObjectMeta = clusterv1beta2.ObjectMeta{
		Labels:      in.ObjectMeta.Labels,
		Annotations: in.ObjectMeta.Annotations,
	}
	return Convert_v1alpha1_ProxmoxMachineSpec_To_v1alpha2_ProxmoxMachineSpec(&in.Spec, &out.Spec, s)
}

func Convert_v1alpha2_ProxmoxMachineTemplateResource_To_v1alpha1_ProxmoxMachineTemplateResource(in *v1alpha2.ProxmoxMachineTemplateResource, out *ProxmoxMachineTemplateResource, s conversion.Scope) error {
	out.ObjectMeta = clusterv1beta1.ObjectMeta{
		Labels:      in.ObjectMeta.Labels,
		Annotations: in.ObjectMeta.Annotations,
	}
	return Convert_v1alpha2_ProxmoxMachineSpec_To_v1alpha1_ProxmoxMachineSpec(&in.Spec, &out.Spec, s)
}

// Convert_v1alpha2_ProxmoxMachineSpec_To_v1alpha1_ProxmoxMachineSpec handles
// the lossy conversion of ProxmoxMachineSpec from v1alpha2 to v1alpha1.
// The FailureDomain field is intentionally dropped (it does not exist in v1alpha1
// and is restored from annotation on ConvertTo).
func Convert_v1alpha2_ProxmoxMachineSpec_To_v1alpha1_ProxmoxMachineSpec(in *v1alpha2.ProxmoxMachineSpec, out *ProxmoxMachineSpec, s conversion.Scope) error {
	return autoConvert_v1alpha2_ProxmoxMachineSpec_To_v1alpha1_ProxmoxMachineSpec(in, out, s)
}

func Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1beta1.ObjectMeta, out *clusterv1beta2.ObjectMeta, s conversion.Scope) error {
	if err := clusterv1beta1.Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in, out, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1alpha1_VirtualMachineCloneSpec_To_v1alpha2_VirtualMachineCloneSpec(in *VirtualMachineCloneSpec, out *v1alpha2.VirtualMachineCloneSpec, s conversion.Scope) error {
	err := autoConvert_v1alpha1_VirtualMachineCloneSpec_To_v1alpha2_VirtualMachineCloneSpec(in, out, s)
	// Target is honored as a single item slice in ConvertTo.
	return err
}

// //
// helpers
// //

func Convert_v1alpha2_NetName_To_string(in *v1alpha2.NetName, out *string, s conversion.Scope) error {
	*out = string(*in)
	return nil
}

func Convert_string_To_v1alpha2_NetName(in *string, out *v1alpha2.NetName, s conversion.Scope) error {
	*out = v1alpha2.NetName(*in)
	return nil
}

func Convert_Slice_v1alpha2_NetName_To_Slice_string(in *[]v1alpha2.NetName, out *[]string, s conversion.Scope) error {
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

func Convert_Slice_string_To_Slice_v1alpha2_NetName(in *[]string, out *[]v1alpha2.NetName, s conversion.Scope) error {
	if in != nil {
		*out = make([]v1alpha2.NetName, 0, len(*in))
		for _, str := range *in {
			var n v1alpha2.NetName
			err := Convert_string_To_v1alpha2_NetName(&str, &n, s)
			if err != nil {
				return err
			}

			*out = append(*out, n)
		}
	}

	return nil
}

func getNetByName(nets []v1alpha2.NetworkDevice, name string) int {
	for i, net := range nets {
		if string(net.Name) == name {
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

// Convert_v1beta1_Condition_To_v1_Condition is the conversion stub required by conversion-gen
// to convert clusterv1beta1.Condition (v1beta1) to metav1.Condition (v1).
func Convert_v1beta1_Condition_To_v1_Condition(in *clusterv1beta1.Condition, out *metav1.Condition, s conversion.Scope) error {
	out.Type = string(in.Type)
	out.Status = metav1.ConditionStatus(in.Status)
	out.LastTransitionTime = in.LastTransitionTime
	out.Reason = in.Reason
	out.Message = in.Message

	if len(out.Reason) == 0 {
		out.Reason = "APIConversionReason"
	}
	if len(out.Message) == 0 {
		out.Message = "API Conversion"
	}

	return nil
}

// Convert_v1_Condition_To_v1beta1_Condition is the conversion stub required by conversion-gen
// to convert metav1.Condition (v1) to clusterv1beta1.Condition (v1beta1).
func Convert_v1_Condition_To_v1beta1_Condition(in *metav1.Condition, out *clusterv1beta1.Condition, s conversion.Scope) error {
	out.Type = clusterv1beta1.ConditionType(in.Type)
	out.Status = corev1.ConditionStatus(in.Status)
	out.LastTransitionTime = in.LastTransitionTime
	out.Reason = in.Reason
	out.Message = in.Message

	if len(out.Reason) == 0 {
		out.Reason = "APIConversionReason"
	}
	if len(out.Message) == 0 {
		out.Message = "API Conversion"
	}

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
func normalizeProxmoxMachineSpec(spec *v1alpha2.ProxmoxMachineSpec) {
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
