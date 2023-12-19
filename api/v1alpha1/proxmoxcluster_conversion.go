package v1alpha1

import (
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this DOCluster to the Hub version (v1beta1).
func (src *ProxmoxCluster) ConvertTo(dstRaw conversion.Hub) error { // nolint
	//dst := dstRaw.(*infrav1.ProxmoxCluster)
	//if err := Convert_v1alpha4_DOCluster_To_v1beta1_DOCluster(src, dst, nil); err != nil {
	//	return err
	//}

	// Manually restore data from annotations
	restored := &infrav1.ProxmoxCluster{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *ProxmoxCluster) ConvertFrom(srcRaw conversion.Hub) error { // nolint
	//src := srcRaw.(*infrav1.ProxmoxCluster)
	//if err := Convert_v1beta1_DOCluster_To_v1alpha4_DOCluster(src, dst, nil); err != nil {
	//	return err
	//}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this DOClusterList to the Hub version (v1beta1).
func (src *ProxmoxClusterList) ConvertTo(dstRaw conversion.Hub) error { // nolint
	//dst := dstRaw.(*infrav1.ProxmoxClusterList)
	//return Convert_v1alpha4_DOClusterList_To_v1beta1_DOClusterList(src, dst, nil)
	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *ProxmoxClusterList) ConvertFrom(srcRaw conversion.Hub) error { // nolint
	//src := srcRaw.(*infrav1.ProxmoxClusterList)
	//return Convert_v1beta1_DOClusterList_To_v1alpha4_DOClusterList(src, dst, nil)
	return nil
}
