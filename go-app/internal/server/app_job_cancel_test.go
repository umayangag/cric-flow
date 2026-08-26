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

func TestClearJobCancel_ForgetsOnlyItsOwnLane(t *testing.T) {
	t.Parallel()
	app := &App{}
	_, computeCancel := context.WithCancel(context.Background())
	dataCtx, dataCancel := context.WithCancel(context.Background())
	app.SetJobCancel(pipelinesvc.LaneCompute, computeCancel)
	app.SetJobCancel(pipelinesvc.LaneData, dataCancel)

	app.ClearJobCancel(pipelinesvc.LaneCompute)
	assert.Equal(t, 1, app.CancelJobsInLanes(), "the finished compute job leaves only the download")
	assert.Error(t, dataCtx.Err())
}

// A nil App is the zero value handlers may hold in tests; none of these may panic.
func TestJobCancels_NilAppIsInert(t *testing.T) {
	t.Parallel()
	var app *App
	assert.NotPanics(t, func() {
		app.SetJobCancel(pipelinesvc.LaneData, func() {})
		app.ClearJobCancel(pipelinesvc.LaneData)
		assert.Zero(t, app.CancelJobsInLanes())
	})
}
