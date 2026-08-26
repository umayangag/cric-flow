package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	cancelled, err := StopRun(context.Background(), "test", nil, cancelJob, cancelMigration)
	require.NoError(t, err)
	assert.Equal(t, 1, cancelled)
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

	_, err := StopRun(context.Background(), "test", []Lane{LaneData}, cancelJob, cancelMigration)
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
	cancelled, err := StopRun(context.Background(), "user", nil, cancelJob, cancelMigration)
	require.Error(t, err)
	assert.Zero(t, cancelled)
}

func TestCommandsInLanes(t *testing.T) {
	t.Parallel()
	assert.Nil(t, CommandsInLanes(nil), "no lanes means no filter, not an empty filter")
	assert.Equal(t, Steps().CommandsInLane(LaneCompute), CommandsInLanes([]Lane{LaneCompute}))

	both := CommandsInLanes([]Lane{LaneCompute, LaneData})
	assert.Len(t, both, len(Steps().All()), "the two lanes together cover every step")
}
