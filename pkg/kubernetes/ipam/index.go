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
