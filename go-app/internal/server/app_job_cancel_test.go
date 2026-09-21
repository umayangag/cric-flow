package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// TestJobCancels_AreKeyedByLane is the regression guard for the bug the second lane
// made reachable. With one slot, starting a download while training ran overwrote the
// training job's cancel func: the training run became uncancellable and Stop killed
// the download instead. Both facts are asserted here because either alone would still
// let the other regress.
func TestJobCancels_AreKeyedByLane(t *testing.T) {
	t.Parallel()
	app := &App{}

	computeCtx, computeCancel := context.WithCancel(context.Background())
	dataCtx, dataCancel := context.WithCancel(context.Background())
	app.SetJobCancel(pipelinesvc.LaneCompute, computeCancel)
	app.SetJobCancel(pipelinesvc.LaneData, dataCancel)

	require.Equal(t, 1, app.CancelJobsInLanes(pipelinesvc.LaneData))
	assert.Error(t, dataCtx.Err(), "the data-lane job must be cancelled")
	assert.NoError(t, computeCtx.Err(), "cancelling a download must not abandon a training run")

	require.Equal(t, 1, app.CancelJobsInLanes(pipelinesvc.LaneCompute))
	assert.Error(t, computeCtx.Err())
}

func TestCancelJobsInLanes_WithNoLanesCancelsEverything(t *testing.T) {
	t.Parallel()
	app := &App{}
	computeCtx, computeCancel := context.WithCancel(context.Background())
	dataCtx, dataCancel := context.WithCancel(context.Background())
	app.SetJobCancel(pipelinesvc.LaneCompute, computeCancel)
	app.SetJobCancel(pipelinesvc.LaneData, dataCancel)

	assert.Equal(t, 2, app.CancelJobsInLanes())
	assert.Error(t, computeCtx.Err())
	assert.Error(t, dataCtx.Err())
	assert.Zero(t, app.CancelJobsInLanes(), "a second Stop has nothing left to cancel")
}

func TestSetJobCancel_ReleaseForgetsOnlyItsOwnLane(t *testing.T) {
	t.Parallel()
	app := &App{}
	_, computeCancel := context.WithCancel(context.Background())
	dataCtx, dataCancel := context.WithCancel(context.Background())
	releaseCompute := app.SetJobCancel(pipelinesvc.LaneCompute, computeCancel)
	app.SetJobCancel(pipelinesvc.LaneData, dataCancel)

	releaseCompute()
	assert.Equal(t, 1, app.CancelJobsInLanes(), "the finished compute job leaves only the download")
	assert.Error(t, dataCtx.Err())
}

// TestSetJobCancel_SecondJobInTheLaneCannotDeregisterTheFirst pins GO-06's second half.
// Registering was `a.jobCancels[lane] = cancel` and releasing was
// `delete(a.jobCancels, lane)`, neither of which asked whose registration it was
// touching. A second job that reached the lane — which the check-then-insert lock could
// allow — replaced the running job's cancel func on the way in and deleted the
// survivor's on the way out, so Stop cancelled neither. The interleaving is forced here
// rather than raced for: the second job registers and releases while the first is still
// running.
func TestSetJobCancel_SecondJobInTheLaneCannotDeregisterTheFirst(t *testing.T) {
	t.Parallel()
	app := &App{}
	firstCtx, firstCancel := context.WithCancel(context.Background())
	secondCtx, secondCancel := context.WithCancel(context.Background())

	releaseFirst := app.SetJobCancel(pipelinesvc.LaneCompute, firstCancel)
	releaseSecond := app.SetJobCancel(pipelinesvc.LaneCompute, secondCancel)
	releaseSecond()

	require.Equal(t, 1, app.CancelJobsInLanes(pipelinesvc.LaneCompute),
		"the lane must still hold the job that claimed it")
	assert.Error(t, firstCtx.Err(), "Stop must cancel the job that is actually running")
	assert.NoError(t, secondCtx.Err(), "the refused job is not the one Stop is aimed at")

	releaseFirst()
	assert.Zero(t, app.CancelJobsInLanes(pipelinesvc.LaneCompute))
}

// A nil App is the zero value handlers may hold in tests; none of these may panic.
func TestJobCancels_NilAppIsInert(t *testing.T) {
	t.Parallel()
	var app *App
	assert.NotPanics(t, func() {
		release := app.SetJobCancel(pipelinesvc.LaneData, func() {})
		release()
		assert.Zero(t, app.CancelJobsInLanes())
	})
}
