package opsstatus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorseStatus(t *testing.T) {
	testCases := []struct {
		a, b string
		want string
	}{
		{"ok", "ok", "ok"},
		{"ok", "stale", "stale"},
		{"ok", "missing", "missing"},
		{"ok", "unknown", "unknown"},
		{"stale", "ok", "stale"},
		{"stale", "stale", "stale"},
		{"stale", "missing", "missing"},
		{"stale", "unknown", "unknown"},
		{"missing", "ok", "missing"},
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
