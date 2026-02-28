package exportqueries

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithSeqEnabled_IsSeqEnabled(t *testing.T) {
	t.Parallel()

	t.Run("nil context stores value", func(t *testing.T) {
		ctx := WithSeqEnabled(nil, true)
		require.True(t, IsSeqEnabled(ctx))
	})

	t.Run("false roundtrip", func(t *testing.T) {
		ctx := WithSeqEnabled(context.Background(), false)
		require.False(t, IsSeqEnabled(ctx))
	})

	t.Run("true roundtrip", func(t *testing.T) {
		ctx := WithSeqEnabled(context.Background(), true)
		require.True(t, IsSeqEnabled(ctx))
	})

	t.Run("default context returns false", func(t *testing.T) {
		require.False(t, IsSeqEnabled(context.Background()))
	})

	t.Run("nil context without WithSeqEnabled returns false", func(t *testing.T) {
		require.False(t, IsSeqEnabled(nil))
	})
}
