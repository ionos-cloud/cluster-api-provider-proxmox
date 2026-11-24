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

package consts

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGetIPAMInClusterAPIGroup(t *testing.T) {
	group := GetIPAMInClusterAPIGroup()
	require.NotNil(t, group)
	require.Equal(t, "ipam.cluster.x-k8s.io", *group, "TypedLocalObjectReference.APIGroup holds the group without a version")
	require.NotContains(t, *group, "/")

	// Two calls hand out distinct pointers to equal values: callers must compare values.
	require.NotSame(t, group, GetIPAMInClusterAPIGroup())
	require.Equal(t, *group, *GetIPAMInClusterAPIGroup())

	gv, err := schema.ParseGroupVersion(*group + "/v1alpha2")
	require.NoError(t, err)
	require.Equal(t, GetIPAMInClusterAPIVersion(), gv.Group)
}

func TestGetPoolKinds(t *testing.T) {
	require.Equal(t, "InClusterIPPool", GetInClusterIPPoolKind())
	require.Equal(t, "GlobalInClusterIPPool", GetGlobalInClusterIPPoolKind())
	require.Equal(t, InClusterIPPool, GetInClusterIPPoolKind())
	require.Equal(t, GlobalInClusterIPPool, GetGlobalInClusterIPPoolKind())
}
