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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// ProxmoxMachineTemplate is a type that implements
// the interfaces from the admission package.
type ProxmoxMachineTemplate struct{}

// SetupWebhookWithManager sets up the webhook with the
// custom interfaces.
func (p *ProxmoxMachineTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.ProxmoxMachineTemplate{}).
		WithDefaulter(p).
		WithValidator(p).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha2-proxmoxmachinetemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachinetemplates,versions=v1alpha2,name=validation.proxmoxmachinetemplate.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1alpha2-proxmoxmachinetemplate,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachinetemplates,versions=v1alpha2,name=default.proxmoxmachinetemplate.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

// ValidateCreate implements the creation validation function.
func (p *ProxmoxMachineTemplate) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	var allErrs field.ErrorList

	machine, ok := obj.(*infrav1.ProxmoxMachineTemplate)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxMachine but got %T", obj))
	}

	if machine.Spec.Template.Spec.ProviderID != nil {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "template", "spec", "providerID"), "cannot be set in templates"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(machine.GroupVersionKind().GroupKind(), machine.Name, allErrs)
}

// ValidateUpdate implements the update validation function.
func (p *ProxmoxMachineTemplate) ValidateUpdate(_ context.Context, old, newObj runtime.Object) (warnings admission.Warnings, err error) {
	var allErrs field.ErrorList

	oldMachineTemplate, ok := old.(*infrav1.ProxmoxMachineTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an ProxmoxMachineTemplate old object but got a %T", old))
	}

	newMachineTemplate, ok := newObj.(*infrav1.ProxmoxMachineTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an ProxmoxMachineTemplate new object but got a %T", newObj))
	}

	if !reflect.DeepEqual(newMachineTemplate.Spec, oldMachineTemplate.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "ProxmoxMachineTemplate is immutable"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(newMachineTemplate.GroupVersionKind().GroupKind(), newMachineTemplate.Name, allErrs)
}

// ValidateDelete implements the deletion validation function.
func (p *ProxmoxMachineTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}

// Default implements the defaulting (mutating) webhook for ProxmoxMachineTemplates.
func (p *ProxmoxMachineTemplate) Default(_ context.Context, _ runtime.Object) error {
	return nil
}
