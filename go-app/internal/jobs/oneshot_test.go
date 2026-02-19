package jobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	jobs "github.com/umayangag/cric-flow/go-app/internal/jobs"
)

// TestOneShotSource_Next_Table follows the gold-standard table-driven style
// (external test package, Arrange → Act → Assert, require assertions).
func TestOneShotSource_Next_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func(ctx context.Context) *jobs.OneShotSource
	type assertFn func(t *testing.T, ids []int64, ok bool, err error)

	cases := []struct {
		name    string
		batch   int
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name:  "nil receiver short-circuits",
			batch: 5,
			arrange: func(context.Context) *jobs.OneShotSource {
				return nil
			},
			assert: func(t *testing.T, ids []int64, ok bool, err error) {
				require.NoError(t, err)
				require.False(t, ok)
				require.Nil(t, ids)
			},
		},
		{
			name:  "first call yields single id (batch ignored)",
			batch: 10,
			arrange: func(context.Context) *jobs.OneShotSource {
				return jobs.NewOneShotSource(42)
			},
			assert: func(t *testing.T, ids []int64, ok bool, err error) {
				require.NoError(t, err)
				require.True(t, ok)
				require.Len(t, ids, 1)
				require.Equal(t, int64(42), ids[0])
			},
		},
		{
			name:  "second call after first is exhausted",
			batch: 1,
			arrange: func(_ context.Context) *jobs.OneShotSource {
				s := jobs.NewOneShotSource(7)
				// Exhaust the one-shot by calling once before the Act phase
				_, _, _ = s.Next(context.Background(), 1)
				return s
			},
			assert: func(t *testing.T, ids []int64, ok bool, err error) {
				require.NoError(t, err)
				require.False(t, ok)
				require.Nil(t, ids)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// Arrange
			s := tc.arrange(ctx)

			// Act
			ids, ok, err := s.Next(ctx, tc.batch)

			// Assert
			tc.assert(t, ids, ok, err)
		})
	}
}
