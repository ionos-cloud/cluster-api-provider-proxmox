/*
Copyright 2025 IONOS Cloud.

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

package proxmoxtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFindVMTemplatesByTags_Return(t *testing.T) {
	m := NewMockClient(t)
	m.EXPECT().FindVMTemplatesByTags(mock.Anything, []string{"tag"}, []string{"node1"}, false, "exact").
		Run(func(_ context.Context, _ []string, _ []string, _ bool, _ string) {}).
		Return(map[string]int32{"node1": 42}, nil)

	result, err := m.FindVMTemplatesByTags(context.Background(), []string{"tag"}, []string{"node1"}, false, "exact")
	require.NoError(t, err)
	require.Equal(t, map[string]int32{"node1": 42}, result)
}

func TestFindVMTemplatesByTags_RunAndReturn(t *testing.T) {
	m := NewMockClient(t)
	m.EXPECT().FindVMTemplatesByTags(mock.Anything, []string{"a", "b"}, []string{"node1", "node2"}, true, "exact").
		RunAndReturn(func(_ context.Context, tags []string, nodes []string, local bool, policy string) (map[string]int32, error) {
			require.Equal(t, []string{"a", "b"}, tags)
			require.Equal(t, []string{"node1", "node2"}, nodes)
			require.True(t, local)
			require.Equal(t, "exact", policy)
			return map[string]int32{"node1": 7, "node2": 8}, nil
		})

	result, err := m.FindVMTemplatesByTags(context.Background(), []string{"a", "b"}, []string{"node1", "node2"}, true, "exact")
	require.NoError(t, err)
	require.Equal(t, map[string]int32{"node1": 7, "node2": 8}, result)
}
