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

package consts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
)

// TestGetIPAMInClusterAPIGroup asserts the value used for
// TypedLocalObjectReference.APIGroup is the bare API group, not the
// group/version string. The Kubernetes API contract for APIGroup requires the
// group only (e.g. "ipam.cluster.x-k8s.io").
func TestGetIPAMInClusterAPIGroup(t *testing.T) {
	got := GetIPAMInClusterAPIGroup()
	require.NotNil(t, got)
	require.Equal(t, ipamicv1.GroupVersion.Group, *got)
	require.NotContains(t, *got, "/", "APIGroup must not contain a version component")
}

// TestGetIPAMInClusterAPIVersion asserts the group used for TypeMeta matching is
// the bare group, consistent with GetIPAMInClusterAPIGroup.
func TestGetIPAMInClusterAPIVersion(t *testing.T) {
	require.Equal(t, ipamicv1.GroupVersion.Group, GetIPAMInClusterAPIVersion())
	require.Equal(t, *GetIPAMInClusterAPIGroup(), GetIPAMInClusterAPIVersion())
	require.False(t, strings.Contains(GetIPAMInClusterAPIVersion(), "/"))
}
