/*
Copyright 2023-2024 IONOS Cloud.

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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
)

// ProxmoxMachine is a type that implements
// the interfaces from the admission package.
type ProxmoxMachine struct{}

// SetupWebhookWithManager sets up the webhook with the
// custom interfaces.
func (p *ProxmoxMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.ProxmoxMachine{}).
		WithValidator(p).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-proxmoxmachine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines,versions=v1alpha1,name=validation.proxmoxmachine.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

// ValidateCreate implements the creation validation function.
func (p *ProxmoxMachine) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	machine, ok := obj.(*infrav1.ProxmoxMachine)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxMachine but got %T", obj))
	}

	err = validateNetworks(machine)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("cannot create proxmox machine %s", machine.GetName()))
		return warnings, err
	}

	return warnings, nil
}

// ValidateUpdate implements the update validation function.
func (p *ProxmoxMachine) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newMachine, ok := newObj.(*infrav1.ProxmoxMachine)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxMachine but got %T", newObj))
	}

	err = validateNetworks(newMachine)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("cannot update proxmox machine %s", newMachine.GetName()))
		return warnings, err
	}

	return warnings, nil
}

// ValidateDelete implements the deletion validation function.
func (p *ProxmoxMachine) ValidateDelete(_ context.Context, _ runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}

func validateNetworks(machine *infrav1.ProxmoxMachine) error {
	gk, name := machine.GroupVersionKind().GroupKind(), machine.GetName()

	if machine.Spec.Network.Default != nil {
		err := validateNetworkDevice(machine.Spec.Network.Default)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "default", "mtu"), machine.Spec.Network.Default, err.Error()),
				})
		}
	}

	for i := range machine.Spec.Network.AdditionalDevices {
		err := validateNetworkDevice(&machine.Spec.Network.AdditionalDevices[i].NetworkDevice)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "additionalDevices", fmt.Sprint(i), "mtu"), machine.Spec.Network.Default, err.Error()),
				})
		}
	}

	return nil
}

func validateNetworkDevice(device *infrav1.NetworkDevice) error {
	if device.MTU == nil {
		return nil
	}

	// special value '1' to inherit the MTU value from the underlying bridge
	if *device.MTU == 1 {
		return nil
	}

	if *device.MTU > 999 {
		return nil
	}

	return fmt.Errorf("mtu must be at least 1000 or 1, but was %d", *device.MTU)
}
