// File: pkg/proxmox/proxmoxtest/mock_client_test.go
// Package: proxmoxtest

package proxmoxtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFindVMTemplateByTags_Return(t *testing.T) {
	m := NewMockClient(t)
	m.EXPECT().FindVMTemplateByTags(mock.Anything, []string{"tag"}, "subset").
		Run(func(_ context.Context, _ []string, _ string) {}).
		Return("tmpl", int32(42), nil)

	id, node, err := m.FindVMTemplateByTags(context.Background(), []string{"tag"}, "subset")
	require.NoError(t, err)
	require.Equal(t, "tmpl", id)
	require.Equal(t, int32(42), node)
}

func TestFindVMTemplateByTags_RunAndReturn(t *testing.T) {
	m := NewMockClient(t)
	m.EXPECT().FindVMTemplateByTags(mock.Anything, []string{"a", "b"}, "exact").
		RunAndReturn(func(_ context.Context, tags []string, policy string) (string, int32, error) {
			require.Equal(t, []string{"a", "b"}, tags)
			require.Equal(t, "exact", policy)
			return "tmpl2", int32(7), nil
		})

	id, node, err := m.FindVMTemplateByTags(context.Background(), []string{"a", "b"}, "exact")
	require.NoError(t, err)
	require.Equal(t, "tmpl2", id)
	require.Equal(t, int32(7), node)
}
