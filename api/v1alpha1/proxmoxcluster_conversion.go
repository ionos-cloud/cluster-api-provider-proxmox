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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// ConvertTo converts this ProxmoxCluster to the Hub version (v1alpha2).
func (src *ProxmoxCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.ProxmoxCluster)
	if err := Convert_v1alpha1_ProxmoxCluster_To_v1alpha2_ProxmoxCluster(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data from annotations
	restored := &infrav1.ProxmoxCluster{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Restore lossy fields
	dst.Spec.ZoneConfigs = restored.Spec.ZoneConfigs
	dst.Status.InClusterZoneRef = restored.Status.InClusterZoneRef

	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.ExternalManagedControlPlane, ok, restored.Spec.ExternalManagedControlPlane, &dst.Spec.ExternalManagedControlPlane)

	if dst.Spec.CloneSpec != nil {
		Convert_string_To_Pointer_string(src.Spec.CloneSpec.VirtualIPNetworkInterface,
			ok,
			getRestoredVirtualIPNetworkInterface(&restored.Spec, ok),
			&dst.Spec.CloneSpec.VirtualIPNetworkInterface,
		)

		if len(dst.Spec.CloneSpec.ProxmoxClusterClassSpec) > 0 {

			for i := range dst.Spec.CloneSpec.ProxmoxClusterClassSpec {
				var srcSpec *ProxmoxMachineSpec

				machineType := dst.Spec.CloneSpec.ProxmoxClusterClassSpec[i].MachineType
				if src.Spec.CloneSpec != nil {
					cp, found := src.Spec.CloneSpec.ProxmoxMachineSpec[machineType]
					if !found {
						continue
					}
					srcSpec = &cp
				}

				if ok && restored.Spec.CloneSpec != nil &&
					i < len(restored.Spec.CloneSpec.ProxmoxClusterClassSpec) {
					restoreProxmoxMachineSpec(srcSpec,
						&dst.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec,
						&restored.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec, ok)
				} else {
					// No restored data - use conversion helper with ok=false
					dstSpec := &dst.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec
					clusterv1.Convert_int32_To_Pointer_int32(srcSpec.NumCores, false, nil, &dstSpec.NumCores)
					clusterv1.Convert_int32_To_Pointer_int32(srcSpec.NumSockets, false, nil, &dstSpec.NumSockets)
					clusterv1.Convert_int32_To_Pointer_int32(srcSpec.MemoryMiB, false, nil, &dstSpec.MemoryMiB)
				}

				// Normalize each machine spec in CloneSpec
				normalizeProxmoxMachineSpec(&dst.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec)

			}

		}

	}

	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restored.Status.Ready, &dst.Status.Ready)

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *ProxmoxCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.ProxmoxCluster)
	if err := Convert_v1alpha2_ProxmoxCluster_To_v1alpha1_ProxmoxCluster(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this ProxmoxClusterList to the Hub version (v1alpha2).
func (src *ProxmoxClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.ProxmoxClusterList)
	return Convert_v1alpha1_ProxmoxClusterList_To_v1alpha2_ProxmoxClusterList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *ProxmoxClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.ProxmoxClusterList)
	return Convert_v1alpha2_ProxmoxClusterList_To_v1alpha1_ProxmoxClusterList(src, dst, nil)
}
