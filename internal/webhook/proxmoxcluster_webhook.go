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
	"net/netip"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"go4.org/netipx"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

var _ admission.CustomValidator = &ProxmoxCluster{}

// ProxmoxCluster is a type that implements
// the interfaces from the admission package.
type ProxmoxCluster struct{}

// SetupWebhookWithManager sets up the webhook with the
// custom interfaces.
func (p *ProxmoxCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.ProxmoxCluster{}).
		WithValidator(p).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha2-proxmoxcluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters,versions=v1alpha2,name=validation.proxmoxcluster.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

// ValidateCreate implements the creation validation function.
func (*ProxmoxCluster) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	cluster, ok := obj.(*infrav1.ProxmoxCluster)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxCluster but got %T", obj))
	}

	if hasNoIPPoolConfig(cluster) {
		err = errors.New("proxmox cluster must define at least one IP pool config")
		warnings = append(warnings, fmt.Sprintf("proxmox cluster must define at least one IP pool config %s", cluster.GetName()))
		return warnings, err
	}

	if err := validateControlPlaneEndpoint(cluster); err != nil {
		warnings = append(warnings, fmt.Sprintf("cannot create proxmox cluster %s", cluster.GetName()))
		return warnings, err
	}

	return warnings, nil
}

// ValidateDelete implements the deletion validation function.
func (*ProxmoxCluster) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements the update validation function.
func (*ProxmoxCluster) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newCluster, ok := newObj.(*infrav1.ProxmoxCluster)
	if !ok {
		return warnings, apierrors.NewBadRequest(fmt.Sprintf("expected a ProxmoxCluster but got %T", newCluster))
	}

	if err := validateControlPlaneEndpoint(newCluster); err != nil {
		warnings = append(warnings, fmt.Sprintf("cannot update proxmox cluster %s", newCluster.GetName()))
		return warnings, err
	}

	return warnings, nil
}

func validateControlPlaneEndpoint(cluster *infrav1.ProxmoxCluster) error {
	// Skipping the validation of the Control Plane endpoint in case of externally managed Control Plane:
	// the Cluster API Control Plane provider will eventually provide the LB.
	if cluster.Spec.ExternalManagedControlPlane {
		return nil
	}

	gk, name := cluster.GroupVersionKind().GroupKind(), cluster.GetName()

	endpoint := cluster.Spec.ControlPlaneEndpoint.Host

	addr, err := netip.ParseAddr(endpoint)

	/*
	   No further validation is done on hostnames. Checking DNS records
	   incures a lot of complexity. To list a few of the problems:
	    - DNS TTL will lead to incorrect results
	    - IP addresses can be PTR records
	    - Both A and AAAA records would need checking
	    - A record can have multiple entries, each of which need to be checked
	    - A valid record can start with _, but that is not a valid hostname
	    - ...
	   Most importantly, cluster-api does not validate controlPlaneEndpoint
	   at all.
	*/
	match := isHostname(endpoint)
	if match {
		return nil
	}

	if err != nil {
		return apierrors.NewInvalid(
			gk,
			name,
			field.ErrorList{
				field.Invalid(
					field.NewPath("spec", "controlplaneEndpoint"), endpoint, "provided endpoint address is not a valid IP or FQDN"),
			})
	}

	// IPv4
	if cluster.Spec.IPv4Config != nil {
		set, err := buildSetFromAddresses(cluster.Spec.IPv4Config.Addresses)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "IPv4Config", "addresses"), cluster.Spec.IPv4Config.Addresses, "provided addresses are not valid IP addresses, ranges or CIDRs"),
				})
		}

		if set.Contains(addr) {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "IPv4Config", "addresses"), cluster.Spec.IPv4Config.Addresses, "addresses may not contain the endpoint IP"),
				})
		}
	}

	// IPV6
	if cluster.Spec.IPv6Config != nil {
		set6, err := buildSetFromAddresses(cluster.Spec.IPv6Config.Addresses)
		if err != nil {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "IPv6Config", "addresses"), cluster.Spec.IPv6Config.Addresses, "provided addresses are not valid IP addresses, ranges or CIDRs"),
				})
		}

		if set6.Contains(addr) {
			return apierrors.NewInvalid(
				gk,
				name,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "IPv6Config", "addresses"), cluster.Spec.IPv6Config.Addresses, "addresses may not contain the endpoint IP"),
				})
		}
	}

	return nil
}

func buildSetFromAddresses(addresses []string) (*netipx.IPSet, error) {
	builder := netipx.IPSetBuilder{}

	for _, address := range addresses {
		switch {
		case strings.Contains(address, "-"):
			ipRange, err := netipx.ParseIPRange(address)
			if err != nil {
				return nil, err
			}
			builder.AddRange(ipRange)
		case strings.Contains(address, "/"):
			ipPref, err := netip.ParsePrefix(address)
			if err != nil {
				return nil, err
			}
			builder.AddPrefix(ipPref)
		default:
			ipAddress, err := netip.ParseAddr(address)
			if err != nil {
				return nil, err
			}

			builder.Add(ipAddress)
		}
	}

	set, err := builder.IPSet()
	if err != nil {
		return nil, err
	}

	return set, nil
}

func hasNoIPPoolConfig(cluster *infrav1.ProxmoxCluster) bool {
	return cluster.Spec.IPv4Config == nil && cluster.Spec.IPv6Config == nil
}

func isHostname(h string) bool {
	// shortname is up to 253 bytes long
	shortname := `([a-z0-9]{1,253}|[a-z0-9][a-z0-9-]{1,251}[a-z0-9])`
	// hostname is optional in a domain
	hostname := `([a-z0-9]{1,63}|[a-z0-9][a-z0-9-]{1,61}[a-z0-9]\.)?`
	domain := `((([a-z0-9]{1,63}|[a-z0-9][a-z0-9-]{1,61}[a-z0-9])\.)+?[a-z]{2,63})`

	// make hostname match case insensitive, match complete string
	hostmatch := `(?i)^(` + shortname + `|` + hostname + domain + `)$`

	match, _ := regexp.Match(hostmatch, []byte(h))
	return match
}
