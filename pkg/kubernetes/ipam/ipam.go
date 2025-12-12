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
	"reflect"
	"regexp"

	infrav1alpha2 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

const (
	globalInClusterIPPool = "GlobalInClusterIPPool"
	inClusterIPPool       = "InClusterIPPool"
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

// InClusterPoolFormat returns the name of the `InClusterIPPool` for a given cluster.
func InClusterPoolFormat(cluster *infrav1.ProxmoxCluster, format string) string {
	return fmt.Sprintf("%s-%s-icip", cluster.GetName(), format)
}

// GetInClusterPools returns the IPPools belonging to the ProxmoxCluster.
func (h *Helper) GetInClusterPools(ctx context.Context, moxm *infrav1.ProxmoxMachine) (map[string]*ipamicv1.InClusterIPPool, error) {
	pools := map[string]*ipamicv1.InClusterIPPool{}
	namespace := moxm.ObjectMeta.Namespace

	// cluster, _ := util.GetClusterFromMetadata(ctx, h.ctrlClient, machine.ObjectMeta)
	clusterName := moxm.ObjectMeta.Labels["cluster.x-k8s.io/cluster-name"]
	proxmoxCluster := &infrav1alpha2.ProxmoxCluster{}

	h.ctrlClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, proxmoxCluster)

	// TODO: Per ZONE IPPoolRefs
	for _, poolRef := range proxmoxCluster.Status.InClusterIPPoolRef {
		pool := &ipamicv1.InClusterIPPool{}
		h.ctrlClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: poolRef.Name}, pool)
		// There's no way of telling if a pool is ipv4 or ipv6 except for parsing it.
		// cluster-api-in-cluster-ipam keeps the pool functions to tag a pool ipv4/ipv6 internal,
		// so we need to reinvent the wheel here.
		re := regexp.MustCompile(`^[^-/]+`)
		ipString := re.FindString(pool.Spec.Addresses[0])
		ip, _ := netip.ParseAddr(ipString)

		if ip.Is4() {
			pools["ipv4"] = pool
		} else if ip.Is6() {
			pools["ipv6"] = pool
		}
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
	// ipv4
	if h.cluster.Spec.IPv4Config != nil {
		ipv4Config := h.cluster.Spec.IPv4Config

		v4Pool := &ipamicv1.InClusterIPPool{
			TypeMeta: metav1.TypeMeta{
				APIVersion: ipamicv1.GroupVersion.String(),
				// Thank you ipamic for making InClusterIPPoolKind private
				Kind: reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      InClusterPoolFormat(h.cluster, infrav1.IPV4Format),
				Namespace: h.cluster.GetNamespace(),
				Annotations: func() map[string]string {
					if ipv4Config.Metric != nil {
						return map[string]string{"metric": fmt.Sprint(*ipv4Config.Metric)}
					}
					return nil
				}(),
			},
			Spec: ipamicv1.InClusterIPPoolSpec{
				Addresses: ipv4Config.Addresses,
				Prefix:    int(ipv4Config.Prefix),
				Gateway:   ipv4Config.Gateway,
			},
		}

		desired := v4Pool.DeepCopy()
		_, err := controllerutil.CreateOrUpdate(ctx, h.ctrlClient, v4Pool, func() error {
			v4Pool.Spec = desired.Spec

			if v4Pool.ObjectMeta.Annotations == nil && desired.ObjectMeta.Annotations != nil {
				v4Pool.ObjectMeta.Annotations = make(map[string]string)
			}
			if desired.ObjectMeta.Annotations != nil {
				v4Pool.ObjectMeta.Annotations["metric"] = desired.ObjectMeta.Annotations["metric"]
			}
			if v4Pool.ObjectMeta.Annotations != nil && desired.ObjectMeta.Annotations == nil {
				delete(v4Pool.ObjectMeta.Annotations, "metric")
			}

			// set the owner reference to the cluster
			return controllerutil.SetControllerReference(h.cluster, v4Pool, h.ctrlClient.Scheme())
		})
		if err != nil {
			return err
		}
	}

	// ipv6
	if h.cluster.Spec.IPv6Config != nil {
		v6Pool := &ipamicv1.InClusterIPPool{
			TypeMeta: metav1.TypeMeta{
				APIVersion: ipamicv1.GroupVersion.String(),
				// Thank you ipamic for making InClusterIPPoolKind private
				Kind: reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      InClusterPoolFormat(h.cluster, infrav1.IPV6Format),
				Namespace: h.cluster.GetNamespace(),
				Annotations: func() map[string]string {
					if h.cluster.Spec.IPv6Config.Metric != nil {
						return map[string]string{"metric": fmt.Sprint(*h.cluster.Spec.IPv6Config.Metric)}
					}
					return nil
				}(),
			},
			Spec: ipamicv1.InClusterIPPoolSpec{
				Addresses: h.cluster.Spec.IPv6Config.Addresses,
				Prefix:    int(h.cluster.Spec.IPv6Config.Prefix),
				Gateway:   h.cluster.Spec.IPv6Config.Gateway,
			},
		}

		desired := v6Pool.DeepCopy()
		_, err := controllerutil.CreateOrUpdate(ctx, h.ctrlClient, v6Pool, func() error {
			v6Pool.Spec = desired.Spec

			if v6Pool.ObjectMeta.Annotations == nil && desired.ObjectMeta.Annotations != nil {
				v6Pool.ObjectMeta.Annotations = make(map[string]string)
			}
			if desired.ObjectMeta.Annotations != nil {
				v6Pool.ObjectMeta.Annotations["metric"] = desired.ObjectMeta.Annotations["metric"]
			}
			if v6Pool.ObjectMeta.Annotations != nil && desired.ObjectMeta.Annotations == nil {
				delete(v6Pool.ObjectMeta.Annotations, "metric")
			}

			// set the owner reference to the cluster
			return controllerutil.SetControllerReference(h.cluster, v6Pool, h.ctrlClient.Scheme())
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// GetDefaultInClusterIPPool attempts to retrieve the `InClusterIPPool`
// which is managed by the cluster.
func (h *Helper) GetDefaultInClusterIPPool(ctx context.Context, format string) (*ipamicv1.InClusterIPPool, error) {
	return h.GetInClusterIPPool(ctx, &corev1.TypedLocalObjectReference{
		Name: InClusterPoolFormat(h.cluster, format),
	})
}

// GetInClusterIPPool attempts to retrieve the referenced `InClusterIPPool`.
func (h *Helper) GetInClusterIPPool(ctx context.Context, ref *corev1.TypedLocalObjectReference) (*ipamicv1.InClusterIPPool, error) {
	out := &ipamicv1.InClusterIPPool{}
	err := h.ctrlClient.Get(ctx, client.ObjectKey{Namespace: h.cluster.GetNamespace(), Name: ref.Name}, out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// GetGlobalInClusterIPPool attempts to retrieve the referenced `GlobalInClusterIPPool`.
func (h *Helper) GetGlobalInClusterIPPool(ctx context.Context, ref *corev1.TypedLocalObjectReference) (*ipamicv1.GlobalInClusterIPPool, error) {
	out := &ipamicv1.GlobalInClusterIPPool{}
	err := h.ctrlClient.Get(ctx, client.ObjectKey{Name: ref.Name}, out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// GetIPPoolAnnotations attempts to retrieve the annotations of an ippool from an ipaddress object.
func (h *Helper) GetIPPoolAnnotations(ctx context.Context, ipAddress *ipamv1.IPAddress) (map[string]string, error) {
	if ipAddress == nil {
		return nil, errors.New("no IPAddress object provided")
	}

	poolRef := ipAddress.Spec.PoolRef
	var annotations map[string]string
	var err error

	key := &corev1.TypedLocalObjectReference{
		Name: poolRef.Name,
	}

	if poolRef.Kind == inClusterIPPool {
		ipPool, err := h.GetInClusterIPPool(ctx, key)
		annotations = ipPool.ObjectMeta.Annotations
		if err != nil {
			return nil, err
		}
	} else if poolRef.Kind == globalInClusterIPPool {
		ipPool, err := h.GetGlobalInClusterIPPool(ctx, key)
		if err != nil {
			return nil, err
		}
		annotations = ipPool.ObjectMeta.Annotations
	}
	// If neither of these kinds are matched, this is a test case,
	// therefore no action is to be taken.

	return annotations, err
}

// CreateIPAddressClaim creates an IPAddressClaim for a given object.
// TODO: remove.
func (h *Helper) CreateIPAddressClaim(ctx context.Context, owner client.Object, device, format, clusterNameLabel string, ref *corev1.TypedLocalObjectReference) error {
	var gvk schema.GroupVersionKind
	key := client.ObjectKey{
		Namespace: owner.GetNamespace(),
		Name:      owner.GetName(),
	}
	suffix := infrav1.DefaultSuffix

	switch {
	case device == infrav1.DefaultNetworkDevice && ref == nil:
		pool, err := h.GetDefaultInClusterIPPool(ctx, format)
		if err != nil {
			return errors.Wrapf(err, "unable to find inclusterpool for cluster %s", h.cluster.Name)
		}
		key.Name = pool.GetName()
		gvk, err = gvkForObject(pool, h.ctrlClient.Scheme())
		if err != nil {
			return err
		}
	case ref.Kind == inClusterIPPool:
		pool, err := h.GetInClusterIPPool(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "unable to find inclusterpool for cluster %s", h.cluster.Name)
		}
		key.Name = pool.GetName()
		gvk, err = gvkForObject(pool, h.ctrlClient.Scheme())
		if err != nil {
			return err
		}
	case ref.Kind == globalInClusterIPPool:
		pool, err := h.GetGlobalInClusterIPPool(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "unable to find global inclusterpool for cluster %s", h.cluster.Name)
		}
		key.Name = pool.GetName()
		gvk, err = gvkForObject(pool, h.ctrlClient.Scheme())
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("unsupported pool type %s", ref.Kind)
	}

	// Ensures that the claim has a reference to the cluster of the VM to
	// support pausing reconciliation.
	labels := map[string]string{
		clusterv1.ClusterNameLabel: clusterNameLabel,
	}

	desired := &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", owner.GetName(), device, suffix),
			Namespace: owner.GetNamespace(),
			Labels:    labels,
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(gvk.Group),
				Kind:     gvk.Kind,
				Name:     key.Name,
			},
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, h.ctrlClient, desired, func() error {
		// set the owner reference to the cluster
		return controllerutil.SetControllerReference(owner, desired, h.ctrlClient.Scheme())
	})

	return err
}

// CreateIPAddressClaimV2 creates an IPAddressClaim for a given object.
func (h *Helper) CreateIPAddressClaimV2(ctx context.Context, owner client.Object, device string, poolNum int, clusterNameLabel string, ref *corev1.TypedLocalObjectReference) error {
	var gvk schema.GroupVersionKind
	key := client.ObjectKey{
		Namespace: owner.GetNamespace(),
		Name:      owner.GetName(),
	}
	suffix := infrav1.DefaultSuffix

	switch {
	case ref.Kind == inClusterIPPool:
		pool, err := h.GetInClusterIPPool(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "unable to find inclusterpool for cluster %s", h.cluster.Name)
		}
		key.Name = pool.GetName()
		gvk, err = gvkForObject(pool, h.ctrlClient.Scheme())
		if err != nil {
			return err
		}
	case ref.Kind == globalInClusterIPPool:
		pool, err := h.GetGlobalInClusterIPPool(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "unable to find global inclusterpool for cluster %s", h.cluster.Name)
		}
		key.Name = pool.GetName()
		gvk, err = gvkForObject(pool, h.ctrlClient.Scheme())
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("unsupported pool type %s", ref.Kind)
	}

	// Ensures that the claim has a reference to the cluster of the VM to
	// support pausing reconciliation.
	labels := map[string]string{
		clusterv1.ClusterNameLabel: clusterNameLabel,
	}

	desired := &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%02d-%s", owner.GetName(), device, poolNum, suffix),
			Namespace: owner.GetNamespace(),
			Labels:    labels,
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(gvk.Group),
				Kind:     gvk.Kind,
				Name:     key.Name,
			},
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, h.ctrlClient, desired, func() error {
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

		if isOwner {
			out = append(out, addr)
		}
	}

	return out, nil
}

// GetIPAddressByPool attempts to retrieve all IPAddresses belonging to a pool.
func (h *Helper) GetIPAddressByPool(ctx context.Context, poolRef corev1.TypedLocalObjectReference) ([]ipamv1.IPAddress, error) {
	addresses := &ipamv1.IPAddressList{}

	// fieldSelector, err := fields.ParseSelector("spec.poolRef.name=" + poolRef.Name + ",spec.poolRef.kind=" + poolRef.Kind)
	fieldSelector, err := fields.ParseSelector("spec.poolRef.name=" + poolRef.Name)
	if err != nil {
		return nil, err
	}

	listOptions := client.ListOptions{FieldSelector: fieldSelector}
	err = h.ctrlClient.List(ctx, addresses, &listOptions)

	if err != nil {
		return nil, err
	}

	out := make([]ipamv1.IPAddress, 0, len(addresses.Items))
	for _, addr := range addresses.Items {
		groupVersion, _ := schema.ParseGroupVersion(addr.APIVersion)
		if groupVersion.Group != "ipam.cluster.x-k8s.io" {
			continue
		}
		out = append(out, addr)
	}

	return out, nil
}

func gvkForObject(obj runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		return schema.GroupVersionKind{}, errors.Wrapf(err, "unable to determine group version for %T", obj)
	}
	return gvk, err
}
