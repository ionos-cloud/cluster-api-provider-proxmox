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

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// ConvertTo converts this ProxmoxMachine to the Hub version (v1alpha2).
func (src *ProxmoxMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.ProxmoxMachine)
	if err := Convert_v1alpha1_ProxmoxMachine_To_v1alpha2_ProxmoxMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data from annotations
	restored := &infrav1.ProxmoxMachine{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	restoreProxmoxMachineSpec(&src.Spec, &dst.Spec, &restored.Spec, ok)

	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restored.Status.Ready, &dst.Status.Ready)
	if dst.Status.VMStatus != nil && *dst.Status.VMStatus == "" {
		dst.Status.VMStatus = nil
	}

	// Normalize ProxmoxMachineSpec after auto-conversion
	normalizeProxmoxMachineSpec(&dst.Spec)

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *ProxmoxMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.ProxmoxMachine)
	if err := Convert_v1alpha2_ProxmoxMachine_To_v1alpha1_ProxmoxMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this DOClusterList to the Hub version (v1alpha2).
func (src *ProxmoxMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.ProxmoxMachineList)
	return Convert_v1alpha1_ProxmoxMachineList_To_v1alpha2_ProxmoxMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *ProxmoxMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.ProxmoxMachineList)
	return Convert_v1alpha2_ProxmoxMachineList_To_v1alpha1_ProxmoxMachineList(src, dst, nil)
}

func restoreProxmoxMachineSpec(src *ProxmoxMachineSpec, dst *infrav1.ProxmoxMachineSpec, restored *infrav1.ProxmoxMachineSpec, ok bool) {
	if dst.MetadataSettings != nil && restored.MetadataSettings != nil && src.MetadataSettings != nil {
		clusterv1.Convert_bool_To_Pointer_bool(src.MetadataSettings.ProviderIDInjection, ok, restored.MetadataSettings.ProviderIDInjection, &dst.MetadataSettings.ProviderIDInjection)
	}

	clusterv1.Convert_int32_To_Pointer_int32(src.NumCores, ok, restored.NumCores, &dst.NumCores)
	clusterv1.Convert_int32_To_Pointer_int32(src.NumSockets, ok, restored.NumSockets, &dst.NumSockets)
	clusterv1.Convert_int32_To_Pointer_int32(src.MemoryMiB, ok, restored.MemoryMiB, &dst.MemoryMiB)

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
			device := getNetDeviceByName(src.Network.AdditionalDevices, *dst.Network.NetworkDevices[i].Name)
			var name, model, bridge string
			if dst.Network.NetworkDevices[i].Name != nil && *dst.Network.NetworkDevices[i].Name == DefaultNetworkDevice {
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
			Convert_string_To_NetName(name, ok, restored.Network.NetworkDevices[i].Name, &dst.Network.NetworkDevices[i].Name)

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
}
