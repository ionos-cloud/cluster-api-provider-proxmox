/*
Copyright 2025-2026 IONOS Cloud.

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

// Package consts contains global consts.
package consts

import (
	"reflect"

	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
)

// GetGlobalInClusterIPPoolKind returns the kind string of a GlobalInClusterIPPool,
// which is useful for typedlocalobjectreferences.
func GetGlobalInClusterIPPoolKind() string {
	return reflect.ValueOf(ipamicv1.GlobalInClusterIPPool{}).Type().Name()
}

// GetInClusterIPPoolKind returns the kind string of a InClusterIPPool,
// which is useful for typedlocalobjectreferences.
func GetInClusterIPPoolKind() string {
	return reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name()
}

// GetIPAMInClusterAPIGroup returns a pointer to APIGroupVersion as required by
// typedlocalobjectreferences.
func GetIPAMInClusterAPIGroup() *string {
	return ptr.To(ipamicv1.GroupVersion.String())
}

// GetIPAMInClusterAPIVersion returns the APIGroup as required for TypeMeta.
func GetIPAMInClusterAPIVersion() string {
	return ipamicv1.GroupVersion.Group
}

const (
	// GlobalInClusterIPPool is the Global In-Cluster Pool.
	GlobalInClusterIPPool = "GlobalInClusterIPPool"
	// InClusterIPPool is the In-Cluster Pool.
	InClusterIPPool = "InClusterIPPool"
)
