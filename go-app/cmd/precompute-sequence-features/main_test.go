package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRun_DryRun_Table aligns with the gold standard (external pkg, table-driven,
// Arrange → Act → Assert, require assertions).
func TestRun_DryRun_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func() (args []string)
	type assertFn func(t *testing.T, out string, err error)

	cases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name:    "all targets in T20",
			arrange: func() []string { return []string{"-format=T20", "-targets=all", "-dry-run"} },
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				require.Contains(t, out, "precompute (dry-run)")
				require.Contains(t, out, "bat_transitions")
				require.Contains(t, out, "bowl_sequences")
				require.Contains(t, out, "player_windows")
			},
		},
		{
			name:    "selected targets in ODI",
			arrange: func() []string { return []string{"-format=ODI", "-targets=bowl_sequences,overpos", "-dry-run"} },
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				require.Contains(t, out, "format=ODI")
				require.Contains(t, out, "bowl_sequences")
				require.Contains(t, out, "overpos")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer
			args := tc.arrange()
			// Act
			err := run(args, &buf)
			// Assert
			tc.assert(t, buf.String(), err)
		})
	}
}
