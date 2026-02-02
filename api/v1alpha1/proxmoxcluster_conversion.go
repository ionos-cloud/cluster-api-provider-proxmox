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

	if restored.Spec.CloneSpec != nil {
		Convert_string_To_Pointer_string(src.Spec.CloneSpec.VirtualIPNetworkInterface, ok, restored.Spec.CloneSpec.VirtualIPNetworkInterface, &dst.Spec.CloneSpec.VirtualIPNetworkInterface)

		if len(restored.Spec.CloneSpec.ProxmoxClusterClassSpec) > 0 {

			for i := range restored.Spec.CloneSpec.ProxmoxClusterClassSpec {

				spec, found := src.Spec.CloneSpec.ProxmoxMachineSpec[restored.Spec.CloneSpec.ProxmoxClusterClassSpec[i].MachineType]
				if !found {
					continue
				}

				restorePromoxMachineSpec(&spec,
					&dst.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec,
					&restored.Spec.CloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec, ok)

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

// ConvertTo converts this DOClusterList to the Hub version (v1alpha2).
func (src *ProxmoxClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.ProxmoxClusterList)
	return Convert_v1alpha1_ProxmoxClusterList_To_v1alpha2_ProxmoxClusterList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *ProxmoxClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.ProxmoxClusterList)
	return Convert_v1alpha2_ProxmoxClusterList_To_v1alpha1_ProxmoxClusterList(src, dst, nil)
}
