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
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// ConvertTo converts this ProxmoxMachine to the Hub version (v1alpha2).
func (src *ProxmoxMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.ProxmoxMachine)
	if err := Convert_v1alpha1_ProxmoxMachine_To_v1alpha2_ProxmoxMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data from annotations
	restored := &v1alpha2.ProxmoxMachine{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	restoreProxmoxMachineSpec(&src.Spec, &dst.Spec, &restored.Spec, ok)

	// Restore FailureDomain (v1alpha2-only field, set by CAPI machine controller).
	dst.Spec.FailureDomain = restored.Spec.FailureDomain

	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok,
		restored.Status.Initialization.Provisioned,
		&dst.Status.Initialization.Provisioned)

	if dst.Status.VMStatus != nil && *dst.Status.VMStatus == "" {
		dst.Status.VMStatus = nil
	}

	// Normalize ProxmoxMachineSpec after auto-conversion
	normalizeProxmoxMachineSpec(&dst.Spec)

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *ProxmoxMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.ProxmoxMachine)
	if err := Convert_v1alpha2_ProxmoxMachine_To_v1alpha1_ProxmoxMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this DOClusterList to the Hub version (v1alpha2).
func (src *ProxmoxMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.ProxmoxMachineList)
	return Convert_v1alpha1_ProxmoxMachineList_To_v1alpha2_ProxmoxMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *ProxmoxMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.ProxmoxMachineList)
	return Convert_v1alpha2_ProxmoxMachineList_To_v1alpha1_ProxmoxMachineList(src, dst, nil)
}

func restoreProxmoxMachineSpec(src *ProxmoxMachineSpec, dst *v1alpha2.ProxmoxMachineSpec, restored *v1alpha2.ProxmoxMachineSpec, ok bool) {
	if dst.MetadataSettings != nil && restored.MetadataSettings != nil && src.MetadataSettings != nil {
		clusterv1.Convert_bool_To_Pointer_bool(src.MetadataSettings.ProviderIDInjection, ok, restored.MetadataSettings.ProviderIDInjection, &dst.MetadataSettings.ProviderIDInjection)
	}

	clusterv1.Convert_int32_To_Pointer_int32(src.NumCores, ok, restored.NumCores, &dst.NumCores)
	clusterv1.Convert_int32_To_Pointer_int32(src.NumSockets, ok, restored.NumSockets, &dst.NumSockets)
	clusterv1.Convert_int32_To_Pointer_int32(src.MemoryMiB, ok, restored.MemoryMiB, &dst.MemoryMiB)

	// Turn ProxmoxMachineSpec.Target into allowedNodes. in v1alpha1, target will
	// ignore AllowedNodes, so we can literally overwrite these.
	if src.Target != nil {
		dst.AllowedNodes = []string{*src.Target}
	}

	// restore fields that don't exist in v1alpha1
	if dst.Network != nil && restored.Network != nil {
		dst.Network.Zone = restored.Network.Zone

		// Restore network device fields
		for i := range dst.Network.NetworkDevices {
			if i < len(restored.Network.NetworkDevices) {
				dst.Network.NetworkDevices[i].DefaultIPv4 = restored.Network.NetworkDevices[i].DefaultIPv4
				dst.Network.NetworkDevices[i].DefaultIPv6 = restored.Network.NetworkDevices[i].DefaultIPv6
			}
		}
	}

	Convert_string_To_Pointer_string(src.TemplateSource.SourceNode, ok, restored.TemplateSource.SourceNode, &dst.TemplateSource.SourceNode)

	if dst.Network != nil && restored.Network != nil {
		for i := range restored.Network.NetworkDevices {
			device := getNetDeviceByName(src.Network.AdditionalDevices, string(dst.Network.NetworkDevices[i].Name))
			var name, model, bridge string
			if dst.Network.NetworkDevices[i].Name == DefaultNetworkDevice {
				name = DefaultNetworkDevice
				model = ptr.Deref(src.Network.Default.Model, "")
				bridge = src.Network.Default.Bridge
			} else {
				if device != nil {
					name = device.Name
					model = ptr.Deref(device.Model, "")
					bridge = device.Bridge
				}
			}

			Convert_string_To_Pointer_string(model, ok, restored.Network.NetworkDevices[i].Model, &dst.Network.NetworkDevices[i].Model)
			Convert_string_To_Pointer_string(bridge, ok, restored.Network.NetworkDevices[i].Bridge, &dst.Network.NetworkDevices[i].Bridge)
			dst.Network.NetworkDevices[i].Name = v1alpha2.NetName(name)

			if dst.Network.NetworkDevices[i].Routing.Routes != nil {
				for j := range dst.Network.NetworkDevices[i].Routing.Routes {
					if device != nil {
						Convert_string_To_Pointer_string(device.Routing.Routes[j].To, ok, restored.Network.NetworkDevices[i].Routing.Routes[j].To, &dst.Network.NetworkDevices[i].Routing.Routes[j].To)
						Convert_string_To_Pointer_string(device.Routing.Routes[j].Via, ok, restored.Network.NetworkDevices[i].Routing.Routes[j].Via, &dst.Network.NetworkDevices[i].Routing.Routes[j].Via)
					} else {
						Convert_string_To_Pointer_string("", ok, restored.Network.NetworkDevices[i].Routing.Routes[j].To, &dst.Network.NetworkDevices[i].Routing.Routes[j].To)
						Convert_string_To_Pointer_string("", ok, restored.Network.NetworkDevices[i].Routing.Routes[j].Via, &dst.Network.NetworkDevices[i].Routing.Routes[j].Via)
					}
				}
			}
			if dst.Network.NetworkDevices[i].Routing.RoutingPolicy != nil {
				for j := range dst.Network.NetworkDevices[i].Routing.RoutingPolicy {
					if device != nil {
						Convert_string_To_Pointer_string(device.Routing.RoutingPolicy[j].To, ok, restored.Network.NetworkDevices[i].Routing.RoutingPolicy[j].To, &dst.Network.NetworkDevices[i].Routing.RoutingPolicy[j].To)
						Convert_string_To_Pointer_string(device.Routing.RoutingPolicy[j].From, ok, restored.Network.NetworkDevices[i].Routing.RoutingPolicy[j].From, &dst.Network.NetworkDevices[i].Routing.RoutingPolicy[j].From)
					} else {
						Convert_string_To_Pointer_string("", ok, restored.Network.NetworkDevices[i].Routing.RoutingPolicy[j].To, &dst.Network.NetworkDevices[i].Routing.RoutingPolicy[j].To)
						Convert_string_To_Pointer_string("", ok, restored.Network.NetworkDevices[i].Routing.RoutingPolicy[j].From, &dst.Network.NetworkDevices[i].Routing.RoutingPolicy[j].From)
					}
				}
			}

		}
	}

	// NetworkSpec is required, therefore a default interface must be added.
	// Push a dummy interface as a default device.
	if dst.Network == nil {
		dst.Network = &v1alpha2.NetworkSpec{
			NetworkDevices: []v1alpha2.NetworkDevice{{
				Name:        v1alpha2.DefaultNetworkDevice,
				DefaultIPv4: ptr.To(true),
				DefaultIPv6: ptr.To(true),
				Bridge:      ptr.To("vmbr0"),
			}},
		}
	}
}
