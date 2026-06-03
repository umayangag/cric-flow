package opsstatus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/precompute"
)

// helper to extract map[string]any safely
func getMap(m map[string]any, key string, t *testing.T) map[string]any {
	t.Helper()
	v, ok := m[key]
	require.True(t, ok, "key %q not found in map", key)
	mv, ok := v.(map[string]any)
	require.True(t, ok, "key %q is not map[string]any", key)
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
	for _, f := range []string{"TEST", "ODI", "T20I", "T20"} {
		st := getMap(formats, f, t)["status"].(string)
		require.Equal(t, "missing", st, "format %s should be missing", f)
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

	require.NotEmpty(t, sec["last_run"], "expected non-empty last_run")
	formats := getMap(sec, "formats", t)
	require.Equal(t, "ok", getMap(formats, "ODI", t)["status"].(string))
	require.Equal(t, "ok", getMap(formats, "T20", t)["status"].(string))
	require.Equal(t, "missing", getMap(formats, "TEST", t)["status"].(string))
	require.Equal(t, "missing", getMap(formats, "T20I", t)["status"].(string))
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
	require.Equal(t, "stale", getMap(formats, "TEST", t)["status"].(string))
	require.Equal(t, "stale", getMap(formats, "T20I", t)["status"].(string))
	require.Equal(t, "missing", getMap(formats, "ODI", t)["status"].(string))
	require.Equal(t, "missing", getMap(formats, "T20", t)["status"].(string))
}

// TestBuildPrecomputeSection_JSONSerializable ensures the section can be marshalled to JSON.
func TestBuildPrecomputeSection_JSONSerializable(t *testing.T) {
	orig := GetPrecomputeStatus
	t.Cleanup(func() { GetPrecomputeStatus = orig })
	GetPrecomputeStatus = func() precompute.Status { return precompute.Status{} }

	sec := BuildPrecomputeSection(context.Background(), time.Now())
	require.NotNil(t, sec)
}
