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

// Package ipam contains helper functions to create, update and delete
// ipam related resources in a Kubernetes cluster
package ipam

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1" //nolint:staticcheck
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"    //nolint:staticcheck
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	. "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/consts"
)

// Helper provides handling of ipam objects such as, InClusterPool, IPAddressClaim.
type Helper struct {
	ctrlClient client.Client
	cluster    *infrav1.ProxmoxCluster
}

// NewHelper creates new Helper.
func NewHelper(c client.Client, infraCluster *infrav1.ProxmoxCluster) *Helper {
	h := new(Helper)
	h.ctrlClient = c
	h.cluster = infraCluster

	return h
}

// InClusterPoolFormat returns the name of the `InClusterIPPool` for a given cluster and deployment zone.
func InClusterPoolFormat(cluster *infrav1.ProxmoxCluster, zone infrav1.Zone, format string) string {
	if zone != nil {
		return fmt.Sprintf("%s-%s-%s-icip", cluster.GetName(), *zone, format)
	}
	return fmt.Sprintf("%s-%s-icip", cluster.GetName(), format)
}

// IPAddressFormat returns an ipaddress name.
func IPAddressFormat(machineName, proxDeviceName string, offset int, suffix string) string {
	return fmt.Sprintf("%s-%s-%02d-%s", machineName, proxDeviceName, offset, suffix)
}

func isIPv4(ip string) (bool, error) {
	// There's no way of telling if a pool is ipv4 or ipv6 except for parsing it.
	// cluster-api-in-cluster-ipam keeps the pool functions to tag a pool ipv4/ipv6 internal,
	// so we need to reinvent the wheel here.
	re := regexp.MustCompile(`^[^-/]+`)
	ipString := re.FindString(ip)

	netIP, err := netip.ParseAddr(ipString)
	if err != nil {
		return false, err
	}

	return netIP.Is4(), nil
}

// poolFromObjectRef is a local helper to turn any objectRef into a pool,
// The awkward calling convention is due to limitations of golang (no generics on methods,
// no type conversion of constrained types).
func (h *Helper) poolFromObjectRef(ctx context.Context, o interface{}, namespace *string) (client.Object, error) {
	ref := corev1.TypedObjectReference{}

	// Todo: type constrained conversion without panic
	switch t := o.(type) {
	case *corev1.LocalObjectReference:
		// Pool is InClusterIPPool, namespace is equal to the caller.
		value, _ := o.(*corev1.LocalObjectReference)
		ref.APIGroup = GetIpamInClusterAPIGroup()
		ref.Kind = GetInClusterIPPoolKind()
		ref.Name = value.Name

		ref.Namespace = ptr.To(h.cluster.GetNamespace())
	case *corev1.TypedLocalObjectReference:
		value, _ := o.(*corev1.TypedLocalObjectReference)
		ref.APIGroup = GetIpamInClusterAPIGroup()
		ref.Name = value.Name
		ref.Kind = value.Kind

		if namespace != nil {
			ref.Namespace = namespace
		}
	case *corev1.TypedObjectReference:
		// Futureproofing for deployments in different namespaces.
		ref, _ = o.(corev1.TypedObjectReference)
	default:
		return nil, fmt.Errorf("invalid Type: %s", t)
	}

	key := client.ObjectKey{Name: ref.Name}

	var ret client.Object
	var err error
	switch ref.Kind {
	case GetInClusterIPPoolKind():
		key.Namespace = h.cluster.GetNamespace()

		pool := new(ipamicv1.InClusterIPPool)
		err = h.ctrlClient.Get(ctx, key, pool)

		ret = pool
	case GetGlobalInClusterIPPoolKind():
		pool := new(ipamicv1.GlobalInClusterIPPool)
		err = h.ctrlClient.Get(ctx, key, pool)

		ret = pool
	default:
		return nil, errors.Errorf("unsupported pool type %s", ref.Kind)
	}

	if err != nil {
		return nil, err
	}

	return ret, nil
}

// GetInClusterPools returns the IPPools belonging to the ProxmoxCluster relative to its Zone.
// TODO: streamline codeflow (unify GetIPPools).
func (h *Helper) GetInClusterPools(ctx context.Context, moxm *infrav1.ProxmoxMachine) (
	struct {
		Zone infrav1.Zone
		IPv4 *struct {
			Pool    ipamicv1.InClusterIPPool
			PoolRef corev1.TypedLocalObjectReference
		}
		IPv6 *struct {
			Pool    ipamicv1.InClusterIPPool
			PoolRef corev1.TypedLocalObjectReference
		}
	}, error) {
	var pools struct {
		Zone infrav1.Zone
		IPv4 *struct {
			Pool    ipamicv1.InClusterIPPool
			PoolRef corev1.TypedLocalObjectReference
		}
		IPv6 *struct {
			Pool    ipamicv1.InClusterIPPool
			PoolRef corev1.TypedLocalObjectReference
		}
	}

	namespace := moxm.ObjectMeta.Namespace

	zone := moxm.Spec.Network.DefaultNetworkSpec.Zone
	if zone == nil {
		zone = ptr.To("default")
	}

	zoneIndex := slices.IndexFunc(h.cluster.Status.InClusterZoneRef, func(z infrav1.InClusterZoneRef) bool {
		return *zone == *z.Zone
	})

	if zoneIndex == -1 {
		return pools, fmt.Errorf("zone %s not found", *zone)
	}

	pools.Zone = zone
	zoneRef := h.cluster.Status.InClusterZoneRef[zoneIndex]

	if zoneRef.InClusterIPPoolRefV4 != nil {
		o, err := h.poolFromObjectRef(ctx, zoneRef.InClusterIPPoolRefV4, &namespace)
		if err != nil {
			return pools, err
		}
		pool := *o.(*ipamicv1.InClusterIPPool)

		if len(pool.Spec.Addresses) == 0 {
			return pools, fmt.Errorf("InClusterIPPool %s without addresses", pool.Name)
		}
		var poolSpec struct {
			Pool    ipamicv1.InClusterIPPool
			PoolRef corev1.TypedLocalObjectReference
		}
		poolSpec.Pool = pool
		poolSpec.PoolRef = corev1.TypedLocalObjectReference{
			APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
			Name:     pool.Name,
			Kind:     pool.TypeMeta.Kind,
		}
		pools.IPv4 = &poolSpec
	}

	if zoneRef.InClusterIPPoolRefV6 != nil {
		o, err := h.poolFromObjectRef(ctx, zoneRef.InClusterIPPoolRefV6, &namespace)
		if err != nil {
			return pools, err
		}
		pool := *o.(*ipamicv1.InClusterIPPool)

		if len(pool.Spec.Addresses) == 0 {
			return pools, fmt.Errorf("InClusterIPPool %s without addresses", pool.Name)
		}
		var poolSpec struct {
			Pool    ipamicv1.InClusterIPPool
			PoolRef corev1.TypedLocalObjectReference
		}
		poolSpec.Pool = pool
		poolSpec.PoolRef = corev1.TypedLocalObjectReference{
			APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
			Name:     pool.Name,
			Kind:     pool.TypeMeta.Kind,
		}
		pools.IPv6 = &poolSpec
	}

	return pools, nil
}

// ErrMissingAddresses is returned when the cluster IPAM config does not contain any addresses.
var ErrMissingAddresses = errors.New("no valid ip addresses defined for the ip pool")

// CreateOrUpdateInClusterIPPool creates or updates an `InClusterIPPool` which will be
// used by the `cluster-api-ipam-provider-in-cluster` to provide IP addresses for new nodes.
// We also need to create this resource to pre-allocate IP addresses which are already in use
// by Proxmox in order to avoid conflicts.
func (h *Helper) CreateOrUpdateInClusterIPPool(ctx context.Context) error {
	// pre allocate to make the linter happy
	zoneSpecs := make([]infrav1.ZoneConfigSpec, 0, len(h.cluster.Spec.ZoneConfigs)+1)
	zoneSpecs = append(zoneSpecs, infrav1.ZoneConfigSpec{
		Zone:       nil,
		IPv4Config: h.cluster.Spec.IPv4Config,
		IPv6Config: h.cluster.Spec.IPv6Config,
	})

	zoneSpecs = append(zoneSpecs, h.cluster.Spec.ZoneConfigs...)

	for _, zoneSpec := range zoneSpecs {
		for _, poolSpec := range []*infrav1.IPConfigSpec{zoneSpec.IPv4Config, zoneSpec.IPv6Config} {
			if poolSpec == nil {
				continue
			}

			isv4, err := isIPv4(poolSpec.Addresses[0])
			if err != nil {
				return err
			}

			format := infrav1.IPv4Format
			family := infrav1.IPv4Type
			if !isv4 {
				format = infrav1.IPv6Format
				family = infrav1.IPv6Type
			}

			pool := &ipamicv1.InClusterIPPool{
				TypeMeta: metav1.TypeMeta{
					APIVersion: ipamicv1.GroupVersion.String(),
					// Thank you ipamic for making InClusterIPPoolKind private
					Kind: GetInClusterIPPoolKind(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      InClusterPoolFormat(h.cluster, zoneSpec.Zone, format),
					Namespace: h.cluster.GetNamespace(),
					Annotations: func() map[string]string {
						metric := ""
						if i := ptr.Deref(poolSpec.Metric, -1); i >= 0 {
							metric = fmt.Sprintf("%d", *poolSpec.Metric)
						}
						annotations := map[string]string{
							infrav1.ProxmoxIPFamilyAnnotation:      family,
							infrav1.ProxmoxGatewayMetricAnnotation: metric,
						}

						// Field deprecated by prefixed value. We need to retag all
						// annotations before we can remove this.
						if poolSpec.Metric != nil {
							annotations["metric"] = metric
						}
						return annotations
					}(),
					Labels: func() map[string]string {
						if zoneSpec.Zone != nil {
							return map[string]string{infrav1.ProxmoxZoneLabel: *zoneSpec.Zone}
						}
						return map[string]string{infrav1.ProxmoxZoneLabel: "default"}
					}(),
				},
				Spec: ipamicv1.InClusterIPPoolSpec{
					Addresses: poolSpec.Addresses,
					Prefix:    int(poolSpec.Prefix),
					Gateway:   poolSpec.Gateway,
				},
			}

			desired := pool.DeepCopy()
			_, err = controllerutil.CreateOrUpdate(ctx, h.ctrlClient, pool, func() error {
				pool.Spec = desired.Spec

				if pool.ObjectMeta.Annotations == nil && desired.ObjectMeta.Annotations != nil {
					pool.ObjectMeta.Annotations = make(map[string]string)
				}
				if desired.ObjectMeta.Annotations != nil {
					pool.ObjectMeta.Annotations["metric"] = desired.ObjectMeta.Annotations["metric"]
					pool.ObjectMeta.Annotations[infrav1.ProxmoxGatewayMetricAnnotation] =
						desired.ObjectMeta.Annotations[infrav1.ProxmoxGatewayMetricAnnotation]
					// IPFamily of a pool should be immutable, but nothing in ipamic
					// protects a pool from it.
					pool.ObjectMeta.Annotations[infrav1.ProxmoxIPFamilyAnnotation] =
						desired.ObjectMeta.Annotations[infrav1.ProxmoxIPFamilyAnnotation]
				}
				// Deleting annotations no longer happens because we need to store ip family

				// Never update label "node.kubernetes.io/proxmox-zone". It's supposed to be immutable.

				// set the owner reference to the cluster
				return controllerutil.SetControllerReference(h.cluster, pool, h.ctrlClient.Scheme())
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// GetDefaultInClusterIPPool attempts to retrieve the `InClusterIPPool`
// which is managed by the cluster.
func (h *Helper) GetDefaultInClusterIPPool(ctx context.Context, format string) (*ipamicv1.InClusterIPPool, error) {
	return h.GetInClusterIPPool(ctx, corev1.TypedLocalObjectReference{
		APIGroup: GetIpamInClusterAPIGroup(),
		Name:     InClusterPoolFormat(h.cluster, nil, format),
		Kind:     GetInClusterIPPoolKind(),
	})
}

// GetIPPool attempts to retrieve a pool from a reference.
func (h *Helper) GetIPPool(ctx context.Context, ref corev1.TypedLocalObjectReference) (client.Object, error) {
	var ret client.Object
	var err error
	key := client.ObjectKey{Name: ref.Name}

	switch ref.Kind {
	case GetInClusterIPPoolKind():
		key.Namespace = h.cluster.GetNamespace()

		pool := new(ipamicv1.InClusterIPPool)
		err = h.ctrlClient.Get(ctx, key, pool)

		ret = pool
	case GetGlobalInClusterIPPoolKind():
		pool := new(ipamicv1.GlobalInClusterIPPool)
		err = h.ctrlClient.Get(ctx, key, pool)

		ret = pool
	default:
		return nil, errors.Errorf("unsupported pool type %s", ref.Kind)
	}

	if err != nil {
		return nil, err
	}

	return ret, nil
}

// GetInClusterIPPool attempts to retrieve the referenced `InClusterIPPool`.
func (h *Helper) GetInClusterIPPool(ctx context.Context, ref corev1.TypedLocalObjectReference) (*ipamicv1.InClusterIPPool, error) {
	out, err := h.GetIPPool(ctx, ref)
	if out == nil {
		return nil, err
	}

	return out.(*ipamicv1.InClusterIPPool), err
}

// GetGlobalInClusterIPPool attempts to retrieve the referenced `GlobalInClusterIPPool`.
func (h *Helper) GetGlobalInClusterIPPool(ctx context.Context, ref corev1.TypedLocalObjectReference) (*ipamicv1.GlobalInClusterIPPool, error) {
	out, err := h.GetIPPool(ctx, ref)
	if out == nil {
		return nil, err
	}

	return out.(*ipamicv1.GlobalInClusterIPPool), err
}

// GetIPPoolAnnotations attempts to retrieve the annotations of an ippool from an ipaddress object.
func (h *Helper) GetIPPoolAnnotations(ctx context.Context, ipAddress *ipamv1.IPAddress) (map[string]string, error) {
	if ipAddress == nil {
		return nil, errors.New("no IPAddress object provided")
	}

	ipPool, err := h.GetIPPool(ctx, ipAddress.Spec.PoolRef)
	if err != nil {
		return nil, err
	}

	return ipPool.(metav1.Object).GetAnnotations(), nil
}

// CreateIPAddressClaim creates an IPAddressClaim for a given object.
func (h *Helper) CreateIPAddressClaim(ctx context.Context, owner client.Object, device string, poolNum int, ref corev1.TypedLocalObjectReference) error {
	key := client.ObjectKey{
		Namespace: owner.GetNamespace(),
		Name:      owner.GetName(),
	}
	suffix := infrav1.DefaultSuffix

	poolObj, err := h.GetIPPool(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "unable to find %s %s for cluster %s",
			ref.Kind,
			ref.Name,
			owner.GetName(),
		)
	}

	key.Name = poolObj.(metav1.Object).GetName()
	gvk, err := gvkForObject(poolObj, h.ctrlClient.Scheme())
	if err != nil {
		return err
	}

	// Ensures that the claim has a reference to the cluster of the VM to
	// support pausing reconciliation.
	labels := map[string]string{
		clusterv1.ClusterNameLabel: h.cluster.GetName(),
	}

	// Copy Proxmox Zone Label.
	poolLabels := poolObj.(metav1.Object).GetLabels()
	if key, exists := poolLabels[infrav1.ProxmoxZoneLabel]; exists {
		labels[infrav1.ProxmoxZoneLabel] = key
	}

	// Add a reference counter to allow multiple ip addresses per owner.
	annotations := map[string]string{
		infrav1.ProxmoxPoolRefCounterAnnotation: fmt.Sprintf("%d", poolNum),
	}

	// TODO: suffix makes no sense, fmt.Sprintf() needs to be shared with testing
	desired := &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        IPAddressFormat(owner.GetName(), device, poolNum, suffix),
			Namespace:   owner.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(gvk.Group),
				Kind:     gvk.Kind,
				Name:     key.Name,
			},
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, h.ctrlClient, desired, func() error {
		// set the owner reference to the cluster
		return controllerutil.SetControllerReference(owner, desired, h.ctrlClient.Scheme())
	})

	return err
}

// GetIPAddress attempts to retrieve the IPAddress.
func (h *Helper) GetIPAddress(ctx context.Context, key client.ObjectKey) (*ipamv1.IPAddress, error) {
	out := &ipamv1.IPAddress{}
	err := h.ctrlClient.Get(ctx, key, out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// GetIPAddressV2 is the replacement for GetIPAddress.
// TODO: Code cleanup.
func (h *Helper) GetIPAddressV2(ctx context.Context, poolRef corev1.TypedLocalObjectReference, moxm *infrav1.ProxmoxMachine) ([]ipamv1.IPAddress, error) {
	ipAddresses, err := h.GetIPAddressByPool(ctx, poolRef)

	out := make([]ipamv1.IPAddress, 0)
	// fieldSelector, err := fields.ParseSelector("spec.poolRef.name=" + poolRef.Name + ",spec.poolRef.kind=" + poolRef.Kind)
	// fieldSelector, err := fields.ParseSelector("metadata" + poolRef.Name)

	if err != nil {
		return nil, err
	}
	for _, addr := range ipAddresses {
		key := client.ObjectKey{
			Name:      addr.Name,
			Namespace: addr.Namespace,
		}

		// Get the parent to find the owner machine
		// Todo: IPAddressClaim cache in context.
		ipAddressClaim := &ipamv1.IPAddressClaim{}
		err := h.ctrlClient.Get(ctx, key, ipAddressClaim)
		if err != nil {
			return nil, err
		}

		// check if current moxm is in the owner reference
		isOwner, err := controllerutil.HasOwnerReference(ipAddressClaim.OwnerReferences, moxm, h.ctrlClient.Scheme())
		if err != nil {
			return nil, err
		}

		// Forward the offset counter so we can have multiple ip addresses per pool.
		offset, exists := ipAddressClaim.GetAnnotations()[infrav1.ProxmoxPoolRefCounterAnnotation]
		if exists {
			addrAnnotations := addr.GetAnnotations()
			if addrAnnotations == nil {
				addrAnnotations = make(map[string]string)
			}
			addrAnnotations[infrav1.ProxmoxPoolRefCounterAnnotation] = offset
			addr.SetAnnotations(addrAnnotations)
		}

		if isOwner {
			out = append(out, addr)
		}
	}

	return out, nil
}

// GetIPAddressByPool attempts to retrieve all IPAddresses belonging to a pool.
func (h *Helper) GetIPAddressByPool(ctx context.Context, poolRef corev1.TypedLocalObjectReference) ([]ipamv1.IPAddress, error) {
	addresses := &ipamv1.IPAddressList{}

	fieldSelector, err := fields.ParseSelector("spec.poolRef.name=" + poolRef.Name)
	if err != nil {
		return nil, err
	}

	listOptions := client.ListOptions{FieldSelector: fieldSelector}
	err = h.ctrlClient.List(ctx, addresses, &listOptions)

	if err != nil {
		return nil, err
	}

	addresses.Items = slices.DeleteFunc(addresses.Items, func(n ipamv1.IPAddress) bool {
		// Check if we are actually dealing with the right resource kind.
		groupVersion, _ := schema.ParseGroupVersion(n.APIVersion)
		return groupVersion.Group != GetIpamInClusterAPIVersion()
	})

	// Sort result by IPAddress.Name to provide stability to testing.
	slices.SortFunc(addresses.Items, func(a, b ipamv1.IPAddress) int {
		return strings.Compare(a.Name, b.Name)
	})

	return addresses.Items, nil
}

func gvkForObject(obj runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		return schema.GroupVersionKind{}, errors.Wrapf(err, "unable to determine group version for %T", obj)
	}
	return gvk, err
}
