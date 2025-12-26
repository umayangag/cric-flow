package exportqueries

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAppendSeqIfEnabled_Table converts the legacy tests into table-driven AAA style.
func TestAppendSeqIfEnabled_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func() (ctx context.Context, base []string, seq []string)
	type assertFn func(t *testing.T, got []string, base []string, seq []string)

	cases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "bowling: OFF -> unchanged; ON -> base prefix preserved and len grows",
			arrange: func() (context.Context, []string, []string) {
				return context.Background(), []string{"c1", "c2"}, BowlingSeqHeaders()
			},
			assert: func(t *testing.T, got []string, base, seq []string) {
				// OFF path
				require.Len(t, got, len(base))
				// ON path
				ctxOn := WithSeqEnabled(context.Background(), true)
				gotOn := AppendSeqIfEnabled(ctxOn, base, seq)
				require.Len(t, gotOn, len(base)+len(seq))
				for i := range base {
					require.Equalf(t, base[i], gotOn[i], "prefix mismatch at %d", i)
				}
			},
		},
		{
			name: "batting: OFF -> unchanged; ON -> base + seq length",
			arrange: func() (context.Context, []string, []string) {
				return context.Background(), []string{"h1"}, BattingSeqHeaders()
			},
			assert: func(t *testing.T, got []string, base, seq []string) {
				// OFF path
				require.Len(t, got, len(base))
				require.Equal(t, base[0], got[0])
				// ON path
				ctxOn := WithSeqEnabled(context.Background(), true)
				gotOn := AppendSeqIfEnabled(ctxOn, base, seq)
				require.Len(t, gotOn, len(base)+len(seq))
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx, base, seq := tc.arrange()
			// Act
			got := AppendSeqIfEnabled(ctx, base, seq)
			// Assert
			tc.assert(t, got, base, seq)
		})
	}
}
