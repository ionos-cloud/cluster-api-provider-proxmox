/*
Copyright 2023-2025 IONOS Cloud.

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
	"strings"

	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/vmservice"
)

// ProxmoxMachine is a type that implements
// the interfaces from the admission package.
type ProxmoxMachine struct{}

// SetupWebhookWithManager sets up the webhook with the
// custom interfaces.
func (p *ProxmoxMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.ProxmoxMachine{}).
		WithDefaulter(p).
		WithValidator(p).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha2-proxmoxmachine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines,versions=v1alpha2,name=validation.proxmoxmachine.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1
//+kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1alpha2-proxmoxmachine,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines,versions=v1alpha2,name=default.proxmoxmachine.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

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
func (p *ProxmoxMachine) ValidateUpdate(_ context.Context, old, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newMachine, ok := newObj.(*infrav1.ProxmoxMachine)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxMachine but got %T", newObj))
	}

	oldMachine, ok := old.(*infrav1.ProxmoxMachine)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxMachine but got %T", old))
	}
	// tags are immutable
	if !cmp.Equal(newMachine.Spec.Tags, oldMachine.Spec.Tags) {
		return warnings, apierrors.NewBadRequest("tags are immutable")
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

func b2i(b *bool) int {
	if b == nil {
		return 0
	}
	if *b {
		return 1
	}
	return 0
}

func validateNetworks(machine *infrav1.ProxmoxMachine) error {
	if machine.Spec.Network == nil {
		return nil
	}

	gk, name := machine.GroupVersionKind().GroupKind(), machine.GetName()

	sortedNetworks := machine.Spec.Network.NetworkDevices
	slices.SortFunc(sortedNetworks, func(nd1, nd2 infrav1.NetworkDevice) int {
		return strings.Compare(nd1.Name.String(), nd2.Name.String())
	})

	defaultIPv4Count := 0
	defaultIPv6Count := 0
	for i, networkDevice := range sortedNetworks {
		// We can disregard error here, because all names are validated
		// by the CRD.
		offset, _ := vmservice.NetNameToOffset(networkDevice.Name)

		// It is possible to have non consecutive proxmox interface names,
		// but this can only be achieved by adding and then deleting.
		if offset != i {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "networkDevices", fmt.Sprint(i), "name"),
						networkDevice.Name,
						"Non-consecutive proxmox interface name",
					),
				},
			)
		}

		defaultIPv4Count += b2i(networkDevice.DefaultIPv4)
		defaultIPv6Count += b2i(networkDevice.DefaultIPv6)
		if defaultIPv4Count > 1 || defaultIPv6Count > 1 {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "networkDevices", fmt.Sprint(i)),
						networkDevice,
						"More than one default IPv4/IPv6 interface in NetworkDevices",
					),
				})
		}

		err := validateNetworkDeviceMTU(&networkDevice)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "additionalDevices", fmt.Sprint(i), "mtu"),
						networkDevice,
						err.Error()),
				})
		}

		err = validateInterfaceConfigMTU(&networkDevice.InterfaceConfig)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "additionalDevices", fmt.Sprint(i), "linkMtu"),
						networkDevice,
						err.Error()),
				})
		}
		err = validateRoutingPolicy(&networkDevice.InterfaceConfig.RoutingPolicy)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "network", "additionalDevices", fmt.Sprint(i), "routingPolicy"),
						networkDevice, err.Error(),
					),
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
						field.NewPath("spec", "network", "VirtualNetworkDevices", "VRFs", fmt.Sprint(i), "Table"),
						machine.Spec.Network.VirtualNetworkDevices.VRFs[i], err.Error()),
				})
		}
	}

	return nil
}

func validateRoutingPolicy(policies *[]infrav1.RoutingPolicySpec) error {
	for i, policy := range *policies {
		if policy.Table == nil {
			return fmt.Errorf("routing policy [%d] requires a table", i)
		}
	}
	return nil
}

func validateVRFConfigRoutingPolicy(vrf *infrav1.VRFDevice) error {
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

func validateInterfaceConfigMTU(ifconfig *infrav1.InterfaceConfig) error {
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

func validateNetworkDeviceMTU(device *infrav1.NetworkDevice) error {
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

// Default implements the defaulting (mutating) webhook for ProxmoxMachines.
func (p *ProxmoxMachine) Default(_ context.Context, obj runtime.Object) error {
	machine, ok := obj.(*infrav1.ProxmoxMachine)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxMachine but got %T", obj))
	}

	if len(machine.Spec.Network.NetworkDevices) == 0 {
		return nil
	}

	// Patch default networks if they are unset.
	defaultIPv4Count := 0
	defaultIPv6Count := 0

	for _, networkDevice := range machine.Spec.Network.NetworkDevices {
		defaultIPv4Count += b2i(networkDevice.DefaultIPv4)
		defaultIPv6Count += b2i(networkDevice.DefaultIPv6)
	}

	// We guarantee that DefaultNetworkDevice is a valid proxmox network device.
	offset, _ := vmservice.NetNameToOffset(infrav1.DefaultNetworkDevice)
	if defaultIPv4Count == 0 {
		machine.Spec.Network.NetworkDevices[offset].DefaultIPv4 = ptr.To(true)
	}
	if defaultIPv6Count == 0 {
		machine.Spec.Network.NetworkDevices[offset].DefaultIPv6 = ptr.To(true)
	}

	return nil
}
