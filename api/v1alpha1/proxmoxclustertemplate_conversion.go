package v1alpha1

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// ConvertTo converts this ProxmoxClusterTemplate to the Hub version (v1alpha2).
func (src *ProxmoxClusterTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.ProxmoxClusterTemplate)
	if err := Convert_v1alpha1_ProxmoxClusterTemplate_To_v1alpha2_ProxmoxClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	restored := &infrav1.ProxmoxClusterTemplate{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Restore lossy fields
	dst.Spec.Template.Spec.ZoneConfigs = restored.Spec.Template.Spec.ZoneConfigs

	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Template.Spec.ExternalManagedControlPlane, ok, restored.Spec.Template.Spec.ExternalManagedControlPlane, &dst.Spec.Template.Spec.ExternalManagedControlPlane)

	if dst.Spec.Template.Spec.CloneSpec != nil {
		Convert_string_To_Pointer_string(src.Spec.Template.Spec.CloneSpec.VirtualIPNetworkInterface,
			ok,
			getRestoredVirtualIPNetworkInterface(&restored.Spec.Template.Spec, ok),
			&dst.Spec.Template.Spec.CloneSpec.VirtualIPNetworkInterface,
		)

		if len(dst.Spec.Template.Spec.CloneSpec.ProxmoxClusterClassSpec) > 0 {

			for i := range dst.Spec.Template.Spec.CloneSpec.ProxmoxClusterClassSpec {
				var srcSpec *ProxmoxMachineSpec

				machineType := dst.Spec.Template.Spec.CloneSpec.ProxmoxClusterClassSpec[i].MachineType
				if src.Spec.Template.Spec.CloneSpec != nil {
					cp, found := src.Spec.Template.Spec.CloneSpec.ProxmoxMachineSpec[machineType]
					if !found {
						continue
					}
					srcSpec = &cp
				}

				if ok && restored.Spec.Template.Spec.CloneSpec != nil &&
					i < len(restored.Spec.Template.Spec.CloneSpec.ProxmoxClusterClassSpec) {
					restoreProxmoxMachineSpec(srcSpec,
						&dst.Spec.Template.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec,
						&restored.Spec.Template.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec, ok)
				} else {
					// No restored data - use conversion helper with ok=false
					dstSpec := &dst.Spec.Template.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec
					clusterv1.Convert_int32_To_Pointer_int32(srcSpec.NumCores, false, nil, &dstSpec.NumCores)
					clusterv1.Convert_int32_To_Pointer_int32(srcSpec.NumSockets, false, nil, &dstSpec.NumSockets)
					clusterv1.Convert_int32_To_Pointer_int32(srcSpec.MemoryMiB, false, nil, &dstSpec.MemoryMiB)
				}

				// Normalize each machine spec in CloneSpec
				normalizeProxmoxMachineSpec(&dst.Spec.Template.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec)
			}

		}

	}

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *ProxmoxClusterTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.ProxmoxClusterTemplate)
	if err := Convert_v1alpha2_ProxmoxClusterTemplate_To_v1alpha1_ProxmoxClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	// Restore fields which do not survive empty conversion but need to be defined.
	// This is required to keep ClusterClasses v0.1.0 working
	if dst.Spec.Template.Spec.CloneSpec.SSHAuthorizedKeys == nil {
		dst.Spec.Template.Spec.CloneSpec.SSHAuthorizedKeys = []string{}
	}

	// Preserve Hub data on down-conversion.
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this ProxmoxMachineTemplate to the Hub version (v1alpha2).
func (src *ProxmoxClusterTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.ProxmoxClusterTemplateList)
	return Convert_v1alpha1_ProxmoxClusterTemplateList_To_v1alpha2_ProxmoxClusterTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *ProxmoxClusterTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.ProxmoxClusterTemplateList)
	return Convert_v1alpha2_ProxmoxClusterTemplateList_To_v1alpha1_ProxmoxClusterTemplateList(src, dst, nil)
}

func getRestoredVirtualIPNetworkInterface(restored *infrav1.ProxmoxClusterSpec, ok bool) *string {
	if ok && restored.CloneSpec != nil {
		if restored.CloneSpec.VirtualIPNetworkInterface != nil {
			return restored.CloneSpec.VirtualIPNetworkInterface
		}
	}

	return nil
}
