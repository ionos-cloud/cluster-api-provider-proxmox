/*
Copyright 2017 The Kubernetes Authors.

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

// Package util contains a few things forked from cluster-api/util 1.10 to keep v1beta1 support
// see https://main.cluster-api.sigs.k8s.io/developer/providers/migrations/v1.10-to-v1.11#suggested-changes-for-clients-using-cluster-api-go-types :
// Additionally, in v1.11 all the CAPI utils like e.g. IsControlPlaneMachine are now using v1beta2 API types.
// This might lead to additional work for providers to keep using v1beta1 types from CAPI v1.11
// (you have to fork all utils the provider is using from an older CAPI release or replace them with something else).
package util

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

const (
	// CharSet defines the alphanumeric set for random string generation.
	CharSet = "0123456789abcdefghijklmnopqrstuvwxyz"
)

var (
	rnd = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec

	// ErrNoCluster is returned when the cluster
	// label could not be found on the object passed in.
	ErrNoCluster = fmt.Errorf("no %q label present", clusterv1.ClusterNameLabel)
)

// RandomString returns a random alphanumeric string.
func RandomString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = CharSet[rnd.Intn(len(CharSet))]
	}
	return string(result)
}

// IsControlPlaneMachine checks machine is a control plane node.
func IsControlPlaneMachine(machine *clusterv1.Machine) bool {
	_, ok := machine.ObjectMeta.Labels[clusterv1.MachineControlPlaneLabel]
	return ok
}

// GetClusterFromMetadata returns the Cluster object (if present) using the object metadata.
func GetClusterFromMetadata(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*clusterv1.Cluster, error) {
	if obj.Labels[clusterv1.ClusterNameLabel] == "" {
		return nil, errors.WithStack(ErrNoCluster)
	}
	return GetClusterByName(ctx, c, obj.Namespace, obj.Labels[clusterv1.ClusterNameLabel])
}

// GetOwnerCluster returns the Cluster object owning the current resource.
func GetOwnerCluster(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*clusterv1.Cluster, error) {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind != "Cluster" {
			continue
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if gv.Group == clusterv1.GroupVersion.Group {
			return GetClusterByName(ctx, c, obj.Namespace, ref.Name)
		}
	}
	return nil, nil
}

// GetClusterByName finds and return a Cluster object using the specified params.
func GetClusterByName(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.Cluster, error) {
	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}

	if err := c.Get(ctx, key, cluster); err != nil {
		return nil, errors.Wrapf(err, "failed to get Cluster/%s", name)
	}

	return cluster, nil
}

// ClusterToInfrastructureMapFunc returns a handler.ToRequestsFunc that watches for
// Cluster events and returns reconciliation requests for an infrastructure provider object.
func ClusterToInfrastructureMapFunc(ctx context.Context, gvk schema.GroupVersionKind, c client.Client, providerCluster client.Object) handler.MapFunc {
	log := ctrl.LoggerFrom(ctx)
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		cluster, ok := o.(*clusterv1.Cluster)
		if !ok {
			return nil
		}

		// Return early if the InfrastructureRef is nil.
		if cluster.Spec.InfrastructureRef == nil {
			return nil
		}
		gk := gvk.GroupKind()
		// Return early if the GroupKind doesn't match what we expect.
		infraGK := cluster.Spec.InfrastructureRef.GroupVersionKind().GroupKind()
		if gk != infraGK {
			return nil
		}
		providerCluster := providerCluster.DeepCopyObject().(client.Object)
		key := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Spec.InfrastructureRef.Name}

		if err := c.Get(ctx, key, providerCluster); err != nil {
			log.V(4).Info(fmt.Sprintf("Failed to get %T", providerCluster), "err", err)
			return nil
		}

		if annotations.IsExternallyManaged(providerCluster) {
			log.V(4).Info(fmt.Sprintf("%T is externally managed, skipping mapping", providerCluster))
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: cluster.Namespace,
					Name:      cluster.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}

// GetOwnerMachine returns the Machine object owning the current resource.
func GetOwnerMachine(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*clusterv1.Machine, error) {
	for _, ref := range obj.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, err
		}
		if ref.Kind == "Machine" && gv.Group == clusterv1.GroupVersion.Group {
			return GetMachineByName(ctx, c, obj.Namespace, ref.Name)
		}
	}
	return nil, nil
}

// GetMachineByName finds and return a Machine object using the specified params.
func GetMachineByName(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.Machine, error) {
	m := &clusterv1.Machine{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

// MachineToInfrastructureMapFunc returns a handler.ToRequestsFunc that watches for
// Machine events and returns reconciliation requests for an infrastructure provider object.
func MachineToInfrastructureMapFunc(gvk schema.GroupVersionKind) handler.MapFunc {
	return func(_ context.Context, o client.Object) []reconcile.Request {
		m, ok := o.(*clusterv1.Machine)
		if !ok {
			return nil
		}

		gk := gvk.GroupKind()
		// Return early if the GroupKind doesn't match what we expect.
		infraGK := m.Spec.InfrastructureRef.GroupVersionKind().GroupKind()
		if gk != infraGK {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: m.Namespace,
					Name:      m.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}

// HasOwnerRef returns true if the OwnerReference is already in the slice. It matches based on Group, Kind and Name.
func HasOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) bool {
	return indexOwnerRef(ownerReferences, ref) > -1
}

// EnsureOwnerRef makes sure the slice contains the OwnerReference.
// Note: EnsureOwnerRef will update the version of the OwnerReference fi it exists with a different version. It will also update the UID.
func EnsureOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) []metav1.OwnerReference {
	idx := indexOwnerRef(ownerReferences, ref)
	if idx == -1 {
		return append(ownerReferences, ref)
	}
	ownerReferences[idx] = ref
	return ownerReferences
}

// RemoveOwnerRef returns the slice of owner references after removing the supplied owner ref.
// Note: RemoveOwnerRef ignores apiVersion and UID. It will remove the passed ownerReference where it matches Name, Group and Kind.
func RemoveOwnerRef(ownerReferences []metav1.OwnerReference, inputRef metav1.OwnerReference) []metav1.OwnerReference {
	if index := indexOwnerRef(ownerReferences, inputRef); index != -1 {
		return append(ownerReferences[:index], ownerReferences[index+1:]...)
	}
	return ownerReferences
}

// indexOwnerRef returns the index of the owner reference in the slice if found, or -1.
func indexOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) int {
	for index, r := range ownerReferences {
		if referSameObject(r, ref) {
			return index
		}
	}
	return -1
}

// Returns true if a and b point to the same object based on Group, Kind and Name.
func referSameObject(a, b metav1.OwnerReference) bool {
	aGV, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return false
	}

	bGV, err := schema.ParseGroupVersion(b.APIVersion)
	if err != nil {
		return false
	}

	return aGV.Group == bGV.Group && a.Kind == b.Kind && a.Name == b.Name
}
