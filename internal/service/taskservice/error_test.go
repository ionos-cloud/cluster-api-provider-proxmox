package taskservice

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRequeueError(t *testing.T) {
	t.Run("RequeueError with message test and duration 42ns", func(t *testing.T) {
		err := NewRequeueError("test", 42)
		require.Error(t, err)
		require.Equal(t, "test, requeuing after 42ns", err.Error())
	})
}
