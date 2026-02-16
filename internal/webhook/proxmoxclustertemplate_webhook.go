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

// Package webhook contains webhooks for the custom resources.
package webhook

import (
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

var _ admission.CustomValidator = &ProxmoxClusterTemplate{}

// ProxmoxClusterTemplate is a type that implements
// the interfaces from the admission package.
type ProxmoxClusterTemplate struct{}

// SetupWebhookWithManager sets up the webhook with the
// custom interfaces.
func (p *ProxmoxClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.ProxmoxClusterTemplate{}).
		WithValidator(p).
		WithDefaulter(p).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha2-proxmoxclustertemplate,mutating=false,failurePolicy=fail,matchPolicy=Exact,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclustertemplates,versions=v1alpha2,name=validation.proxmoxclustertemplate.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1alpha2-proxmoxclustertemplate,mutating=true,failurePolicy=fail,matchPolicy=Exact,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclustertemplates,versions=v1alpha2,name=default.proxmoxclustertemplate.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

// Default implements the defaulting (mutating) webhook for ProxmoxCluster.
func (p *ProxmoxClusterTemplate) Default(_ context.Context, _ runtime.Object) error {
	return nil
}

// ValidateCreate implements the creation validation function.
func (*ProxmoxClusterTemplate) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	cluster, ok := obj.(*infrav1.ProxmoxClusterTemplate)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxClusterTemplate but got %T", obj))
	}

	if hasNoIPPoolConfig(&cluster.Spec.Template.Spec.ProxmoxClusterSpec) {
		err = errors.New("proxmox cluster must define at least one IP pool config")
		warnings = append(warnings, fmt.Sprintf("proxmox cluster template must define at least one IP pool config %s", cluster.GetName()))
		return warnings, err
	}

	if err := validateControlPlaneEndpoint(&cluster.Spec.Template.Spec.ProxmoxClusterSpec, cluster.GroupVersionKind().GroupKind(), cluster.GetName()); err != nil {
		warnings = append(warnings, fmt.Sprintf("cannot create proxmox cluster template %s", cluster.GetName()))
		return warnings, err
	}

	if err := validateTemplateCloneSpecHasControlPlane(&cluster.Spec.Template.Spec.ProxmoxClusterCloneSpec, cluster.GroupVersionKind().GroupKind(), cluster.GetName()); err != nil {
		warnings = append(warnings, fmt.Sprintf("cannot create proxmox cluster template %s", cluster.GetName()))
		return warnings, err
	}

	return warnings, nil
}

// validateTemplateCloneSpecHasControlPlane validates that if cloneSpec.machineSpec is provided,
// it must contain an entry with machineType "controlPlane".
// This validation is only applied to ProxmoxClusterTemplates.
func validateTemplateCloneSpecHasControlPlane(spec *infrav1.ProxmoxClusterCloneSpec, gk schema.GroupKind, name string) error {
	/*
		if spec.Template.Spec.ProxmoxClusterCloneSpec == nil {
			return nil
		}
	*/

	machineSpecs := spec.ProxmoxClusterClassSpec
	if len(machineSpecs) == 0 {
		return nil
	}

	// listMapKey=machineType ensures uniqueness, so we only need to check existence
	idx := slices.IndexFunc(machineSpecs, func(spec infrav1.ProxmoxClusterClassSpec) bool {
		return spec.MachineType == "controlPlane"
	})

	if idx < 0 {
		return apierrors.NewInvalid(
			gk,
			name,
			field.ErrorList{
				field.Invalid(
					field.NewPath("spec", "cloneSpec", "machineSpec"),
					machineSpecs,
					"machineSpec must contain an entry with machineType 'controlPlane'",
				),
			})
	}

	return nil
}

// ValidateDelete implements the deletion validation function.
func (*ProxmoxClusterTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements the update validation function.
func (*ProxmoxClusterTemplate) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newCluster, ok := newObj.(*infrav1.ProxmoxClusterTemplate)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxClusterTemplate but got %T", newCluster))
	}

	if err := validateControlPlaneEndpoint(&newCluster.Spec.Template.Spec.ProxmoxClusterSpec, newCluster.GroupVersionKind().GroupKind(), newCluster.GetName()); err != nil {
		warnings = append(warnings, fmt.Sprintf("cannot update proxmox cluster %s", newCluster.GetName()))
		return warnings, err
	}

	if err := validateTemplateCloneSpecHasControlPlane(&newCluster.Spec.Template.Spec.ProxmoxClusterCloneSpec, newCluster.GroupVersionKind().GroupKind(), newCluster.GetName()); err != nil {
		warnings = append(warnings, fmt.Sprintf("cannot update proxmox cluster template %s", newCluster.GetName()))
		return warnings, err
	}

	return warnings, nil
}
