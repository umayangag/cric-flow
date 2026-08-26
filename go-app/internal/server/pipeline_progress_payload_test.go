package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/precompute"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// fixedNow anchors elapsed-time assertions.
var fixedNow = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

func testReporter() *progressReporter {
	r := newProgressReporter()
	r.now = func() time.Time { return fixedNow }
	r.autoTuneProgress = func(context.Context) map[string]interface{} {
		return map[string]interface{}{"phase": "fine_tuning"}
	}
	r.precomputeStatus = func() precompute.Status { return precompute.Status{} }
	r.fetchStatus = func() (dataacquire.Progress, string, bool) { return dataacquire.Progress{}, "", false }
	r.extractStatus = func() (dataacquire.ExtractProgress, string, bool) {
		return dataacquire.ExtractProgress{}, "", false
	}
	return r
}

func runningFor(command string, ago time.Duration) tracking.Migration {
	return tracking.Migration{
		Command:   command,
		Args:      json.RawMessage(`{"step":"x"}`),
		StartedAt: fixedNow.Add(-ago),
		Status:    tracking.StatusInProgress,
	}
}

func TestSnapshotReportsEveryRunningStep(t *testing.T) {
	t.Parallel()

	// Two steps in flight at once — the case the old single-slot payload could not
	// express, and the one the data lane and the run-plan executor both produce.
	payload := testReporter().snapshot(context.Background(), []tracking.Migration{
		runningFor("train-batting", 90*time.Second),
		runningFor("cricsheet-import", 5*time.Minute),
	})

	require.True(t, payload.Running)
	require.Len(t, payload.Steps, 2)

	assert.Equal(t, "train_batting", payload.Steps[0].StepID)
	assert.Equal(t, "Train Batting", payload.Steps[0].StepLabel)
	assert.Equal(t, "compute", payload.Steps[0].Lane)
	assert.Equal(t, int64(90), payload.Steps[0].ElapsedSec)

	assert.Equal(t, "import", payload.Steps[1].StepID)
	assert.Equal(t, int64(300), payload.Steps[1].ElapsedSec)
}

func TestSnapshotWithNothingRunning(t *testing.T) {
	t.Parallel()

	payload := testReporter().snapshot(context.Background(), nil)
	assert.False(t, payload.Running)
	assert.Empty(t, payload.Steps)

	// The list must serialise as [] rather than null so clients can iterate blindly.
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{"running":false,"steps":[]}`, string(data))
}

func TestDescribeFallsBackToTheCommandForUnknownSteps(t *testing.T) {
	t.Parallel()

	step := testReporter().describe(context.Background(), runningFor("some-other-job", time.Second))
	assert.Equal(t, "some-other-job", step.StepID)
	assert.Equal(t, "some-other-job", step.StepLabel)
	// An unrecognised job is assumed to touch the database, so it holds the compute lane.
	assert.Equal(t, "compute", step.Lane)
}

func TestDescribeAttachesAutoTuneProgressOnlyToAutoTune(t *testing.T) {
	t.Parallel()

	reporter := testReporter()
	autoTune := reporter.describe(context.Background(), runningFor("ml-auto-tune", time.Second))
	assert.Equal(t, "fine_tuning", autoTune.AutoTune["phase"])

	training := reporter.describe(context.Background(), runningFor("train-win", time.Second))
	assert.Nil(t, training.AutoTune)
}

func TestPrecomputeETA(t *testing.T) {
	t.Parallel()

	migration := runningFor("precompute-features", 10*time.Minute)

	testCases := []struct {
		name   string
		status precompute.Status
		want   *int64
	}{
		{
			name:   "no formats yields no estimate",
			status: precompute.Status{},
			want:   nil,
		},
		{
			name: "current format not in list yields no estimate",
			status: precompute.Status{
				Formats:       []string{"TEST", "ODI"},
				CurrentFormat: "T20",
			},
			want: nil,
		},
		{
			name: "second format uses the observed average of the completed ones",
			status: precompute.Status{
				// 10 min elapsed, 4 min on the current format => 6 min for one
				// completed format. One format remains after this one, and this one
				// has 2 min left: 120 + 360 = 480.
				Formats:         []string{"TEST", "ODI", "T20"},
				CurrentFormat:   "ODI",
				FormatStartedAt: fixedNow.Add(-4 * time.Minute),
			},
			want: ptr(int64(480)),
		},
		{
			name: "last format counts only itself",
			status: precompute.Status{
				Formats:         []string{"TEST", "ODI"},
				CurrentFormat:   "ODI",
				FormatStartedAt: fixedNow.Add(-time.Minute),
			},
			// 9 min for the one completed format, 1 min spent => 8 min left, nothing after.
			want: ptr(int64(480)),
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reporter := testReporter()
			reporter.precomputeStatus = func() precompute.Status { return tc.status }
			_, eta := reporter.precomputeDetail(migration)
			if tc.want == nil {
				assert.Nil(t, eta)
				return
			}
			require.NotNil(t, eta)
			assert.Equal(t, *tc.want, *eta)
		})
	}
}

func ptr[T any](v T) *T { return &v }

// TestFetchProgressReachesTheStream keeps acquisition on the one stream the UI
// already listens to. A second endpoint for download progress would be a second thing
// to connect, reconnect and keep in sync — the plan's "one stream for the UI" applies
// to fetch as much as to the trainers.
func TestFetchProgressReachesTheStream(t *testing.T) {
	t.Parallel()
	eta := int64(42)
	r := testReporter()
	r.fetchStatus = func() (dataacquire.Progress, string, bool) {
		return dataacquire.Progress{
			Downloaded:  1024,
			Total:       4096,
			BytesPerSec: 512,
			ETASec:      &eta,
		}, "https://cricsheet.org/downloads/all_json.zip", true
	}

	payload := r.snapshot(context.Background(), []tracking.Migration{runningFor("dataset-fetch", time.Minute)})
	require.Len(t, payload.Steps, 1)
	step := payload.Steps[0]

	require.NotNil(t, step.Fetch)
	assert.Equal(t, int64(1024), step.Fetch.Downloaded)
	assert.Equal(t, int64(4096), step.Fetch.Total)
	assert.Equal(t, int64(512), step.Fetch.BytesPerSec)
	require.NotNil(t, step.EstimatedSec)
	assert.Equal(t, eta, *step.EstimatedSec)
	assert.Equal(t, "data", step.Lane, "a download must not be reported in the compute lane")
}

// TestFetchWithNoLiveSampleReportsAbsence: a job that has started but not yet written
// a sample has unknown progress, and unknown is not zero. Rendering 0 of 0 bytes reads
// as a stalled transfer.
func TestFetchWithNoLiveSampleReportsAbsence(t *testing.T) {
	t.Parallel()
	r := testReporter()

	payload := r.snapshot(context.Background(), []tracking.Migration{runningFor("dataset-fetch", time.Second)})
	require.Len(t, payload.Steps, 1)
	assert.Nil(t, payload.Steps[0].Fetch)
	assert.Nil(t, payload.Steps[0].EstimatedSec)
}

// TestExtractProgressReachesTheStream keeps extraction on the same single stream as
// everything else, for the same reason fetch is there.
func TestExtractProgressReachesTheStream(t *testing.T) {
	t.Parallel()
	eta := int64(17)
	r := testReporter()
	r.extractStatus = func() (dataacquire.ExtractProgress, string, bool) {
		return dataacquire.ExtractProgress{
			Entries:      1200,
			EntriesTotal: 20000,
			Bytes:        5 << 20,
			ETASec:       &eta,
		}, "all_json.zip", true
	}

	payload := r.snapshot(context.Background(), []tracking.Migration{runningFor("dataset-extract", time.Minute)})
	require.Len(t, payload.Steps, 1)
	step := payload.Steps[0]

	require.NotNil(t, step.Extract)
	assert.Equal(t, 1200, step.Extract.Entries)
	assert.Equal(t, 20000, step.Extract.EntriesTotal)
	require.NotNil(t, step.EstimatedSec)
	assert.Equal(t, eta, *step.EstimatedSec)
	assert.Equal(t, "data", step.Lane)
	assert.Nil(t, step.Fetch, "an extract is not a download")
}
