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
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
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
	r.stepProgress = func(context.Context, string) (map[string]interface{}, error) { return nil, nil }
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

// TestTrainingProgressReachesTheStream is what O-3 exists for: one stream for the UI.
// A second endpoint for training progress would be a second thing to connect,
// reconnect and keep in sync, for a panel that is already listening here.
func TestTrainingProgressReachesTheStream(t *testing.T) {
	t.Parallel()
	r := testReporter()
	var askedFor string
	r.stepProgress = func(_ context.Context, stepID string) (map[string]interface{}, error) {
		askedFor = stepID
		return map[string]interface{}{
			"v": float64(1), "step": "train_batting", "phase": "cv",
			"current": float64(3), "total": float64(5),
			"metrics": map[string]interface{}{"rmse": 24.1},
		}, nil
	}

	payload := r.snapshot(context.Background(), []tracking.Migration{runningFor("train-batting", time.Minute)})
	require.Len(t, payload.Steps, 1)
	step := payload.Steps[0]

	assert.Equal(t, "train_batting", askedFor, "the step id is what ml-service keys progress by")
	require.NotNil(t, step.Training)
	assert.Equal(t, "cv", step.Training["phase"])
	assert.False(t, step.ProgressUnavailable)
}

// TestUnreachableProgressIsUnknownNotFailure: the step may be running perfectly well
// and only the progress link broken. Saying "failed" about a healthy run is worse
// than saying "cannot tell".
func TestUnreachableProgressIsUnknownNotFailure(t *testing.T) {
	t.Parallel()
	r := testReporter()
	r.stepProgress = func(context.Context, string) (map[string]interface{}, error) {
		return nil, pipelinesvc.ErrProgressUnavailable
	}

	payload := r.snapshot(context.Background(), []tracking.Migration{runningFor("train-batting", time.Minute)})
	require.Len(t, payload.Steps, 1)
	step := payload.Steps[0]

	assert.True(t, step.ProgressUnavailable, "the operator can act on a broken link")
	assert.Nil(t, step.Training)
	assert.True(t, payload.Running, "the step is still running; only its progress is unknown")
	assert.Equal(t, "Train Batting", step.StepLabel)
}

// TestNothingPublishedYetIsNotUnavailable distinguishes the two empty panels: a run
// that has not reported yet is not a broken link.
func TestNothingPublishedYetIsNotUnavailable(t *testing.T) {
	t.Parallel()
	r := testReporter()

	payload := r.snapshot(context.Background(), []tracking.Migration{runningFor("train-batting", time.Second)})
	require.Len(t, payload.Steps, 1)

	assert.Nil(t, payload.Steps[0].Training)
	assert.False(t, payload.Steps[0].ProgressUnavailable)
}

// TestStepsRunByGoAppAreNotAskedOfMLService: import, precompute and export run here,
// so asking ml-service about them would be a request per SSE tick for an answer that
// cannot exist.
func TestStepsRunByGoAppAreNotAskedOfMLService(t *testing.T) {
	t.Parallel()
	r := testReporter()
	asked := 0
	r.stepProgress = func(context.Context, string) (map[string]interface{}, error) {
		asked++
		return nil, nil
	}

	for _, command := range []string{"cricsheet-import", "export-dataset", "some-external-command"} {
		_ = r.snapshot(context.Background(), []tracking.Migration{runningFor(command, time.Minute)})
	}
	assert.Zero(t, asked, "only ml-service steps have progress to fetch")
}

func TestTrainingETAUsesTheRunStartNotTheLastEvent(t *testing.T) {
	t.Parallel()
	r := testReporter()
	r.stepProgress = func(context.Context, string) (map[string]interface{}, error) {
		return map[string]interface{}{"current": float64(2), "total": float64(6)}, nil
	}

	// Two of six units done after four minutes: two minutes per unit, four remaining.
	payload := r.snapshot(context.Background(), []tracking.Migration{runningFor("train-batting", 4*time.Minute)})
	require.Len(t, payload.Steps, 1)
	require.NotNil(t, payload.Steps[0].EstimatedSec)
	assert.Equal(t, int64(480), *payload.Steps[0].EstimatedSec)
}

// TestTrainingETAIsAbsentUntilSomethingFinishes: with zero units completed there is no
// observed rate, and a number invented from a default is one nobody should believe.
func TestTrainingETAIsAbsentUntilSomethingFinishes(t *testing.T) {
	t.Parallel()
	for name, progress := range map[string]map[string]interface{}{
		"nothing done": {"current": float64(0), "total": float64(5)},
		"no total":     {"current": float64(2)},
		"already done": {"current": float64(5), "total": float64(5)},
		"no counters":  {"phase": "loading"},
		"total below":  {"current": float64(6), "total": float64(5)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := testReporter()
			r.stepProgress = func(context.Context, string) (map[string]interface{}, error) {
				return progress, nil
			}
			payload := r.snapshot(context.Background(), []tracking.Migration{runningFor("train-batting", time.Minute)})
			require.Len(t, payload.Steps, 1)
			assert.Nil(t, payload.Steps[0].EstimatedSec)
		})
	}
}
