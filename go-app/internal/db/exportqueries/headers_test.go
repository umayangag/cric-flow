package exportqueries

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAppendSeqIfEnabled verifies OFF and ON scenarios as separate table cases
// to keep Arrange-Act-Assert strictly separated per subtest.
func TestAppendSeqIfEnabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		seqEnabled bool
		base       []string
		seqFn      func() []string
	}{
		{
			name:       "bowling headers with seq disabled",
			seqEnabled: false,
			base:       []string{"c1", "c2"},
			seqFn:      BowlingSeqHeaders,
		},
		{
			name:       "bowling headers with seq enabled",
			seqEnabled: true,
			base:       []string{"c1", "c2"},
			seqFn:      BowlingSeqHeaders,
		},
		{name: "batting headers with seq disabled", seqEnabled: false, base: []string{"h1"}, seqFn: BattingSeqHeaders},
		{name: "batting headers with seq enabled", seqEnabled: true, base: []string{"h1"}, seqFn: BattingSeqHeaders},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			if tc.seqEnabled {
				ctx = WithSeqEnabled(ctx, true)
			}
			seq := tc.seqFn()

			// Act
			got := AppendSeqIfEnabled(ctx, tc.base, seq)

			// Assert
			if !tc.seqEnabled {
				// OFF: unchanged
				require.Equal(t, tc.base, got)
				return
			}
			// ON: base prefix preserved and seq appended
			require.Equal(t, len(tc.base)+len(seq), len(got))
			require.Equal(t, tc.base, got[:len(tc.base)])
			require.Equal(t, seq, got[len(tc.base):])
		})
	}
}
