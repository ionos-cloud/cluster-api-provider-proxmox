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

	infrav2 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

// ProxmoxMachine is a type that implements
// the interfaces from the admission package.
type ProxmoxMachine struct{}

// SetupWebhookWithManager sets up the webhook with the
// custom interfaces.
func (p *ProxmoxMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav2.ProxmoxMachine{}).
		WithValidator(p).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha2-proxmoxmachine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines,versions=v1alpha2,name=validation.proxmoxmachine.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

// ValidateCreate implements the creation validation function.
func (p *ProxmoxMachine) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	machine, ok := obj.(*infrav2.ProxmoxMachine)
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
	newMachine, ok := newObj.(*infrav2.ProxmoxMachine)
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

func validateNetworks(machine *infrav2.ProxmoxMachine) error {
	if machine.Spec.Network == nil {
		return nil
	}

	gk, name := machine.GroupVersionKind().GroupKind(), machine.GetName()

	for i := range machine.Spec.Network.NetworkDevices {
		err := validateNetworkDeviceMTU(&machine.Spec.Network.NetworkDevices[i])
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "additionalDevices", fmt.Sprint(i), "mtu"), machine.Spec.Network.NetworkDevices[i], err.Error()),
				})
		}
		err = validateInterfaceConfigMTU(&machine.Spec.Network.NetworkDevices[i].InterfaceConfig)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "additionalDevices", fmt.Sprint(i), "linkMtu"), machine.Spec.Network.NetworkDevices[i], err.Error()),
				})
		}
		err = validateRoutingPolicy(&machine.Spec.Network.NetworkDevices[i].InterfaceConfig.RoutingPolicy)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "additionalDevices", fmt.Sprint(i), "routingPolicy"), machine.Spec.Network.NetworkDevices[i], err.Error()),
				})
		}
	}

	for i := range machine.Spec.Network.VirtualNetworkDevices.VRFs {
		err := validateVRFConfigRoutingPolicy(&machine.Spec.Network.VirtualNetworkDevices.VRFs[i])
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "VirtualNetworkDevices", "VRFs", fmt.Sprint(i), "Table"), machine.Spec.Network.VirtualNetworkDevices.VRFs[i], err.Error()),
				})
		}
	}

	return nil
}

func validateRoutingPolicy(policies *[]infrav2.RoutingPolicySpec) error {
	for i, policy := range *policies {
		if policy.Table == nil {
			return fmt.Errorf("routing policy [%d] requires a table", i)
		}
	}
	return nil
}

func validateVRFConfigRoutingPolicy(vrf *infrav2.VRFDevice) error {
	for _, policy := range vrf.Routing.RoutingPolicy {
		// Netplan will not accept rules not matching the l3mdev table, although
		// there is no technical reason for this limitation.
		if policy.Table != nil {
			if *policy.Table != vrf.Table {
				return fmt.Errorf("VRF %s: device/rule routing table mismatch %d != %d", vrf.Name, vrf.Table, *policy.Table)
			}
		}
	}
	return nil
}

func validateInterfaceConfigMTU(ifconfig *infrav2.InterfaceConfig) error {
	if ifconfig.LinkMTU != nil {
		// We allow MTUs down to 576, but since everything below 1280 breaks IPv6, you
		// should disable the webhook if you really mean it.
		if *ifconfig.LinkMTU > 1279 {
			return nil
		}

		return fmt.Errorf("mtu must be at least 1280, but was %d", *ifconfig.LinkMTU)
	}
	return nil
}

func validateNetworkDeviceMTU(device *infrav2.NetworkDevice) error {
	if device.MTU != nil {
		// special value '1' to inherit the MTU value from the underlying bridge
		if *device.MTU == 1 {
			return nil
		}

		// We allow MTUs down to 576, but since everything below 1280 breaks IPv6, you
		// should disable the webhook if you really mean it.
		if *device.MTU > 1279 {
			return nil
		}

		return fmt.Errorf("mtu must be at least 1280 or 1, but was %d", *device.MTU)
	}

	return nil
}
