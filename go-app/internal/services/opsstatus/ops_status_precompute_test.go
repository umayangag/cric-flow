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
		return precompute.Status{FinishedAt: finished, Phase: precompute.PhaseDone, Formats: []string{"ODI", "T20"}}
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
		return precompute.Status{FinishedAt: finished, Phase: precompute.PhaseDone, Formats: []string{"TEST", "T20I"}}
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

// TestBuildPrecomputeSection_RunStoppedEarly_NotReportedAsFresh is the regression test
// for a cancelled run painting the console green: the run stamps FinishedAt whatever
// happens to it, so the section has to read the outcome, not just the timestamp.
func TestBuildPrecomputeSection_RunStoppedEarly_NotReportedAsFresh(t *testing.T) {
	testCases := []struct {
		name   string
		status precompute.Status
	}{
		{
			name: "cancelled mid-run",
			status: precompute.Status{
				FinishedAt: time.Date(2026, 1, 21, 6, 30, 0, 0, time.UTC),
				Phase:      precompute.PhaseFailed,
				LastError:  "upsert raw stats venue pid=61: context canceled",
				Formats:    []string{"TEST", "ODI", "T20", "T20I"},
			},
		},
		{
			name: "still running",
			status: precompute.Status{
				Running:    true,
				FinishedAt: time.Date(2026, 1, 21, 6, 30, 0, 0, time.UTC),
				Phase:      precompute.PhaseDone,
				Formats:    []string{"TEST", "ODI", "T20", "T20I"},
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			orig := GetPrecomputeStatus
			t.Cleanup(func() { GetPrecomputeStatus = orig })
			GetPrecomputeStatus = func() precompute.Status { return tc.status }
			now := time.Date(2026, 1, 21, 18, 0, 0, 0, time.UTC)

			// No DB in a unit test, so the fallback to the last COMPLETED run finds nothing.
			sec := BuildPrecomputeSection(context.Background(), now)

			require.Empty(t, sec["last_run"], "a run that did not finish is not a last run")
			require.Empty(t, sec["as_of"])
			formats := getMap(sec, "formats", t)
			for _, f := range []string{"TEST", "ODI", "T20I", "T20"} {
				require.Equal(t, "missing", getMap(formats, f, t)["status"].(string),
					"format %s must not be reported as computed", f)
			}
			require.Equal(t, tc.status.LastError, sec["last_error"])
		})
	}
}
