package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorseStatus(t *testing.T) {
	tests := []struct {
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
	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := worseStatus(tt.a, tt.b)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGetCricketFormats(t *testing.T) {
	got := getCricketFormats()
	require.NotEmpty(t, got)
	require.Contains(t, got, "TEST")
	require.Contains(t, got, "ODI")
	require.Contains(t, got, "T20I")
	require.Contains(t, got, "T20")
}
