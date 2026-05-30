package opsstatus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/precompute"

	"github.com/stretchr/testify/require"
)

// helper to extract map[string]any safely
func getMap(m map[string]any, key string, t *testing.T) map[string]any {
	v, ok := m[key]
	require.True(t, ok)
	mv, ok := v.(map[string]any)
	require.True(t, ok)
	return mv
}

func TestBuildPrecomputeSection_NoFinishedRun_AllMissing(t *testing.T) {
	// Arrange
	orig := GetPrecomputeStatus
	t.Cleanup(func() { GetPrecomputeStatus = orig })
	GetPrecomputeStatus = func() precompute.Status {
		return precompute.Status{} // zero FinishedAt
	}
	now := time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC)

	// Act (no DB in unit test, so tracking fallback returns nil → all missing)
	sec := BuildPrecomputeSection(context.Background(), now)

	// Assert
	require.Empty(t, sec["last_run"], "expected empty last_run")
	require.Empty(t, sec["as_of"], "expected empty as_of")
	formats := getMap(sec, "formats", t)
	wantMissing := []string{"TEST", "ODI", "T20I", "T20"}
	for _, f := range wantMissing {
		st := getMap(formats, f, t)["status"].(string)
		require.Equal(t, "missing", st)
	}
}

func TestBuildPrecomputeSection_Today_OkForRanFormats(t *testing.T) {
	orig := GetPrecomputeStatus
	t.Cleanup(func() { GetPrecomputeStatus = orig })
	finished := time.Date(2026, 1, 21, 6, 30, 0, 0, time.UTC)
	GetPrecomputeStatus = func() precompute.Status {
		return precompute.Status{FinishedAt: finished, Formats: []string{"ODI", "T20"}}
	}
	now := time.Date(2026, 1, 21, 18, 0, 0, 0, time.UTC)

	sec := BuildPrecomputeSection(context.Background(), now)
	require.NotEqual(t, "" || sec["as_of"] == "", sec["last_run"])
	formats := getMap(sec, "formats", t)
	require.Equal(t, "ok", st := getMap(formats, "ODI", t)["status"].(string); st)
	require.Equal(t, "ok", st := getMap(formats, "T20", t)["status"].(string); st)
	require.Equal(t, "missing", st := getMap(formats, "TEST", t)["status"].(string); st)
	require.Equal(t, "missing", st := getMap(formats, "T20I", t)["status"].(string); st)
}

func TestBuildPrecomputeSection_Yesterday_StaleForRanFormats(t *testing.T) {
	orig := GetPrecomputeStatus
	t.Cleanup(func() { GetPrecomputeStatus = orig })
	finished := time.Date(2026, 1, 20, 23, 50, 0, 0, time.UTC)
	GetPrecomputeStatus = func() precompute.Status {
		return precompute.Status{FinishedAt: finished, Formats: []string{"TEST", "T20I"}}
	}
	now := time.Date(2026, 1, 21, 0, 10, 0, 0, time.UTC)

	sec := BuildPrecomputeSection(context.Background(), now)
	formats := getMap(sec, "formats", t)
	require.Equal(t, "stale", st := getMap(formats, "TEST", t)["status"].(string); st)
	require.Equal(t, "stale", st := getMap(formats, "T20I", t)["status"].(string); st)
	require.Equal(t, "missing", st := getMap(formats, "ODI", t)["status"].(string); st)
	require.Equal(t, "missing", st := getMap(formats, "T20", t)["status"].(string); st)
}
