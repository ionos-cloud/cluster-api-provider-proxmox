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

	"github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// ConvertTo converts this ProxmoxCluster to the Hub version (v1alpha2).
func (src *ProxmoxCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.ProxmoxCluster)
	if err := Convert_v1alpha1_ProxmoxCluster_To_v1alpha2_ProxmoxCluster(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data from annotations
	restored := &v1alpha2.ProxmoxCluster{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Restore lossy fields
	dst.Spec.ZoneConfigs = restored.Spec.ZoneConfigs
	dst.Status.InClusterZoneRef = restored.Status.InClusterZoneRef
	dst.Status.FailureDomains = restored.Status.FailureDomains

	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.ExternalManagedControlPlane, ok, restored.Spec.ExternalManagedControlPlane, &dst.Spec.ExternalManagedControlPlane)

	// CloneSpec is moved to ProxmoxClusterTemplates. The field serves no function in ProxmoxClusters.

	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restored.Status.Initialization.Provisioned, &dst.Status.Initialization.Provisioned)

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *ProxmoxCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.ProxmoxCluster)
	if err := Convert_v1alpha2_ProxmoxCluster_To_v1alpha1_ProxmoxCluster(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this ProxmoxClusterList to the Hub version (v1alpha2).
func (src *ProxmoxClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.ProxmoxClusterList)
	return Convert_v1alpha1_ProxmoxClusterList_To_v1alpha2_ProxmoxClusterList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *ProxmoxClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.ProxmoxClusterList)
	return Convert_v1alpha2_ProxmoxClusterList_To_v1alpha1_ProxmoxClusterList(src, dst, nil)
}
