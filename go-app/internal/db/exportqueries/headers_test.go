package exportqueries_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	eq "github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)

func TestBowlingSeqHeaders(t *testing.T) {
	t.Parallel()
	got := eq.BowlingSeqHeaders()
	require.Len(t, got, 10)
	require.Equal(t, "bowl_prev_bowler_id", got[0])
	require.Equal(t, "bowl_over_ball6_wkt_rate", got[9])
}

func TestBattingSeqHeaders(t *testing.T) {
	t.Parallel()
	got := eq.BattingSeqHeaders()
	require.Len(t, got, 10)
	require.Equal(t, "bat_prev_batter_id", got[0])
	require.Equal(t, "bat_after_k_dots_boundary_p_k2", got[9])
}

func TestAppendSeqIfEnabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		seqEnabled bool
		base       []string
		seqFn      func() []string
	}{
		{
			name:       "bowling headers with seq disabled",
			seqEnabled: false,
			base:       []string{"c1", "c2"},
			seqFn:      eq.BowlingSeqHeaders,
		},
		{
			name:       "bowling headers with seq enabled",
			seqEnabled: true,
			base:       []string{"c1", "c2"},
			seqFn:      eq.BowlingSeqHeaders,
		},
		{name: "batting headers with seq disabled", seqEnabled: false, base: []string{"h1"}, seqFn: eq.BattingSeqHeaders},
		{name: "batting headers with seq enabled", seqEnabled: true, base: []string{"h1"}, seqFn: eq.BattingSeqHeaders},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			ctx := context.Background()
			if tc.seqEnabled {
				ctx = eq.WithSeqEnabled(ctx, true)
			}
			seq := tc.seqFn()

			// Act
			got := eq.AppendSeqIfEnabled(ctx, tc.base, seq)

			// Assert
			if !tc.seqEnabled {
				require.Equal(t, tc.base, got)
				return
			}
			require.Equal(t, len(tc.base)+len(seq), len(got))
			require.Equal(t, tc.base, got[:len(tc.base)])
			require.Equal(t, seq, got[len(tc.base):])
		})
	}
}
