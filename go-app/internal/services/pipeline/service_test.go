package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noTrainingRunning stands for an ml-service with nothing to stop, which is the answer
// every case below is uninterested in.
func noTrainingRunning(context.Context) ([]string, error) { return nil, nil }

func TestStopRun_CancelsJobsAndMigrations(t *testing.T) {
	t.Parallel()
	var gotLanes []Lane
	var gotCommands []string
	cancelJob := func(lanes ...Lane) int {
		gotLanes = lanes
		return len(lanes)
	}
	cancelMigration := func(_ context.Context, _ string, commands []string) (int, error) {
		gotCommands = commands
		return 1, nil
	}

	outcome, err := StopRun(context.Background(), "test", nil, cancelJob, cancelMigration, noTrainingRunning)
	require.NoError(t, err)
	assert.Equal(t, 1, outcome.Cancelled)
	assert.Empty(t, gotLanes, "no lanes means every lane")
	assert.Nil(t, gotCommands, "no lanes means every command")
}

// TestStopRun_ScopesBothHalvesToTheSameLane is the regression guard for the bug that
// lanes made reachable: the job cancel and the tracking update have to agree on which
// runs they are stopping, or Stop kills one job and records another as cancelled.
func TestStopRun_ScopesBothHalvesToTheSameLane(t *testing.T) {
	t.Parallel()
	var gotLanes []Lane
	var gotCommands []string
	cancelJob := func(lanes ...Lane) int {
		gotLanes = lanes
		return len(lanes)
	}
	cancelMigration := func(_ context.Context, _ string, commands []string) (int, error) {
		gotCommands = commands
		return len(commands), nil
	}

	_, err := StopRun(context.Background(), "test", []Lane{LaneData}, cancelJob, cancelMigration, noTrainingRunning)
	require.NoError(t, err)
	assert.Equal(t, []Lane{LaneData}, gotLanes)
	assert.Equal(t, Steps().CommandsInLane(LaneData), gotCommands)
	assert.NotContains(t, gotCommands, "train-batting", "a data-lane stop must not touch compute runs")
}

func TestStopRun_ReturnsCancelMigrationResult(t *testing.T) {
	t.Parallel()
	cancelJob := func(...Lane) int { return 0 }
	cancelMigration := func(_ context.Context, _ string, _ []string) (int, error) {
		return 0, errors.New("no run to cancel")
	}
	outcome, err := StopRun(context.Background(), "user", nil, cancelJob, cancelMigration, noTrainingRunning)
	require.Error(t, err)
	assert.Zero(t, outcome.Cancelled)
}

// The D-11 guards. A Stop used to cancel go-app's own HTTP request and report
// `{"cancelled": 1}` while `ml.xi.retrain` carried on inside ml-service: the console
// showed the run gone, the compute lane read as free, and the process had to be killed by
// hand. Stopping the training is now part of stopping the run, and its failure is carried
// out rather than swallowed.
func TestStopRun_StopsTheTrainingProcessBeforeCancellingLocally(t *testing.T) {
	t.Parallel()
	var order []string
	cancelJob := func(...Lane) int {
		order = append(order, "cancel_job")
		return 1
	}
	cancelMigration := func(context.Context, string, []string) (int, error) {
		order = append(order, "cancel_migration")
		return 1, nil
	}
	stopTraining := func(context.Context) ([]string, error) {
		order = append(order, "stop_training")
		return []string{"retrain"}, nil
	}

	outcome, err := StopRun(context.Background(), "user", nil, cancelJob, cancelMigration, stopTraining)

	require.NoError(t, err)
	assert.Equal(t, []string{"retrain"}, outcome.TrainingStopped)
	assert.NoError(t, outcome.TrainingErr)
	assert.Equal(t, []string{"stop_training", "cancel_job", "cancel_migration"}, order,
		"the work is stopped before the bookkeeping: once the local job is cancelled the run looks finished from here")
}

func TestStopRun_CarriesOutAFailureToStopTheTraining(t *testing.T) {
	t.Parallel()
	cancelJob := func(...Lane) int { return 1 }
	cancelMigration := func(context.Context, string, []string) (int, error) { return 1, nil }
	stopTraining := func(context.Context) ([]string, error) {
		return nil, errors.New("connection refused")
	}

	outcome, err := StopRun(context.Background(), "user", nil, cancelJob, cancelMigration, stopTraining)

	require.NoError(t, err, "the local cancel succeeded; the remote stop is reported separately")
	require.Error(t, outcome.TrainingErr, "a stop nobody confirmed must not read as a stop")
	assert.Empty(t, outcome.TrainingStopped)
	assert.Equal(t, 1, outcome.Cancelled)
}

// Training runs in the compute lane, so a Stop aimed at the data lane must leave it be —
// the same distinction that keeps a cancelled download from abandoning a retrain.
func TestStopRun_ADataLaneStopDoesNotTouchTraining(t *testing.T) {
	t.Parallel()
	asked := false
	stopTraining := func(context.Context) ([]string, error) {
		asked = true
		return nil, nil
	}

	_, err := StopRun(context.Background(), "user", []Lane{LaneData},
		func(...Lane) int { return 1 },
		func(context.Context, string, []string) (int, error) { return 1, nil },
		stopTraining)

	require.NoError(t, err)
	assert.False(t, asked, "a data-lane stop has no business killing a training run")
}

func TestStopRun_AComputeLaneStopDoesTouchTraining(t *testing.T) {
	t.Parallel()
	asked := false
	stopTraining := func(context.Context) ([]string, error) {
		asked = true
		return []string{"retrain"}, nil
	}

	_, err := StopRun(context.Background(), "user", []Lane{LaneCompute},
		func(...Lane) int { return 1 },
		func(context.Context, string, []string) (int, error) { return 1, nil },
		stopTraining)

	require.NoError(t, err)
	assert.True(t, asked)
}

func TestCommandsInLanes(t *testing.T) {
	t.Parallel()
	assert.Nil(t, CommandsInLanes(nil), "no lanes means no filter, not an empty filter")
	assert.Equal(t, Steps().CommandsInLane(LaneCompute), CommandsInLanes([]Lane{LaneCompute}))

	both := CommandsInLanes([]Lane{LaneCompute, LaneData})
	assert.Len(t, both, len(Steps().All()), "the two lanes together cover every step")
}
