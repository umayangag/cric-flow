package exportqueries_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)

func TestWithSeqEnabled_IsSeqEnabled(t *testing.T) {
	t.Parallel()

	t.Run("nil context stores value", func(t *testing.T) {
		//nolint:staticcheck // SA1012: intentionally testing nil context handling
		ctx := exportqueries.WithSeqEnabled(nil, true)
		require.True(t, exportqueries.IsSeqEnabled(ctx))
	})

	t.Run("false roundtrip", func(t *testing.T) {
		ctx := exportqueries.WithSeqEnabled(context.Background(), false)
		require.False(t, exportqueries.IsSeqEnabled(ctx))
	})

	t.Run("true roundtrip", func(t *testing.T) {
		ctx := exportqueries.WithSeqEnabled(context.Background(), true)
		require.True(t, exportqueries.IsSeqEnabled(ctx))
	})

	t.Run("default context returns false", func(t *testing.T) {
		require.False(t, exportqueries.IsSeqEnabled(context.Background()))
	})

	t.Run("nil context without WithSeqEnabled returns false", func(t *testing.T) {
		//nolint:staticcheck // SA1012: intentionally testing nil context handling
		require.False(t, exportqueries.IsSeqEnabled(nil))
	})
}
