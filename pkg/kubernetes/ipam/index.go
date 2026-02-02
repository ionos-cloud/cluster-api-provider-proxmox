package ipam

import (
	"context"
	"fmt"

	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// IPAddressPoolRefNameField is the path we index upon (the owner reference).
	IPAddressPoolRefNameField = "spec.poolRef.name"
)

// SetupIndexes adds indexes to the cache of a Manager.
func SetupIndexes(ctx context.Context, cache cache.Cache) error {
	return cache.IndexField(ctx, &ipamv1.IPAddress{},
		IPAddressPoolRefNameField,
		IPAddressByPoolRefName,
	)
}

// IPAddressByPoolRefName is the indexing function for our index.
func IPAddressByPoolRefName(o client.Object) []string {
	ip, ok := o.(*ipamv1.IPAddress)
	if !ok {
		panic(fmt.Sprintf("Expected an IPAddress but got a %T", o))
	}
	return []string{ip.Spec.PoolRef.Name}
}
