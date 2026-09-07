package opsstatus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorseStatus(t *testing.T) {
	// The completeness vocabulary only: `stale` went with the freshness buckets P2-1
	// deleted, and nothing rolls a staleness up any more.
	testCases := []struct {
		a, b string
		want string
	}{
		{"ok", "ok", "ok"},
		{"ok", "missing", "missing"},
		{"ok", "unknown", "unknown"},
		{"missing", "ok", "missing"},
		{"missing", "missing", "missing"},
		{"missing", "unknown", "unknown"},
		{"unknown", "missing", "unknown"},
		{"unknown", "unknown", "unknown"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.a+"_"+tc.b, func(t *testing.T) {
			got := worseStatus(tc.a, tc.b)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetCricketFormats(t *testing.T) {
	got := CricketFormatCodes
	require.NotEmpty(t, got)
	require.Contains(t, got, "TEST")
	require.Contains(t, got, "ODI")
	require.Contains(t, got, "T20I")
	require.Contains(t, got, "T20")
}
