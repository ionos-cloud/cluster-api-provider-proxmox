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

// ConvertTo converts this ProxmoxClusterTemplate to the Hub version (v1alpha2).
func (src *ProxmoxClusterTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.ProxmoxClusterTemplate)
	if err := Convert_v1alpha1_ProxmoxClusterTemplate_To_v1alpha2_ProxmoxClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	restored := &v1alpha2.ProxmoxClusterTemplate{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Restore lossy fields
	dst.Spec.Template.Spec.ZoneConfigs = restored.Spec.Template.Spec.ZoneConfigs

	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Template.Spec.ExternalManagedControlPlane, ok, restored.Spec.Template.Spec.ExternalManagedControlPlane, &dst.Spec.Template.Spec.ExternalManagedControlPlane)

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *ProxmoxClusterTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.ProxmoxClusterTemplate)
	if err := Convert_v1alpha2_ProxmoxClusterTemplate_To_v1alpha1_ProxmoxClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	// Fake ProxmoxClusterTemplate v1alpha1 fields, so patches apply for old ClusterClasses.
	dst.Spec.Template.Spec.CloneSpec = &ProxmoxClusterCloneSpec{
		ProxmoxMachineSpec: map[string]ProxmoxMachineSpec{
			"controlPlane": ProxmoxMachineSpec{
				VirtualMachineCloneSpec: VirtualMachineCloneSpec{
					TemplateSource: TemplateSource{
						SourceNode: "pve1",
					},
				},
			},
		},
		SSHAuthorizedKeys:         []string{},
		VirtualIPNetworkInterface: "",
	}

	// Preserve Hub data on down-conversion.
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this ProxmoxMachineTemplate to the Hub version (v1alpha2).
func (src *ProxmoxClusterTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.ProxmoxClusterTemplateList)
	return Convert_v1alpha1_ProxmoxClusterTemplateList_To_v1alpha2_ProxmoxClusterTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *ProxmoxClusterTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.ProxmoxClusterTemplateList)
	return Convert_v1alpha2_ProxmoxClusterTemplateList_To_v1alpha1_ProxmoxClusterTemplateList(src, dst, nil)
}
