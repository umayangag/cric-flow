package runplan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// These exercise the SQL. The unit tests above use a fake Store, which proves the
// executor's decisions but not that data_migrations can hold a plan — an in-flight
// metadata update that also stamped completed_at, or a plan row that LaneBusy counted,
// would pass every one of them and break on the box.
//
//	RUN_DB_TESTS=1 make -C go-app test-db
func guardIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_DB_TESTS") != "1" {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
}

func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../migrations"))
}

func setupPlanDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err, "integration tests need POSTGRES_* pointing at a database")
	require.NoError(t, db.RunMigrations(ctx, migrationsDir()))
	require.NoError(t, db.Exec(ctx, `DELETE FROM data_migrations WHERE command = $1`, PlanCommand))
}

func TestStoreRoundTrip_Integration(t *testing.T) {
	guardIntegration(t)
	setupPlanDB(t)
	ctx := context.Background()
	store := TrackingStore{}

	steps := mustResolve(t, "import", "precompute")
	state := NewState(PlanFull, steps, "2026-08-27T12:00:00Z")

	id, err := store.Create(ctx, PlanFull, state)
	require.NoError(t, err)
	require.NotZero(t, id)

	_, active, running, err := store.Active(ctx)
	require.NoError(t, err)
	require.True(t, running, "a created plan is in flight")
	assert.Equal(t, PlanFull, active.Plan)
	assert.Len(t, active.Steps, 2)
}

// TestSaveDoesNotFinishThePlan_Integration is why UpdateMigrationMetadata exists: the
// status-changing update stamps completed_at, which would mark a plan finished on its
// first step.
func TestSaveDoesNotFinishThePlan_Integration(t *testing.T) {
	guardIntegration(t)
	setupPlanDB(t)
	ctx := context.Background()
	store := TrackingStore{}

	state := NewState(PlanFull, mustResolve(t, "import", "precompute"), "2026-08-27T12:00:00Z")
	id, err := store.Create(ctx, PlanFull, state)
	require.NoError(t, err)

	state.Steps[0].Status = StatusCompleted
	state.Steps[1].Status = StatusRunning
	require.NoError(t, store.Save(ctx, id, state))

	_, active, running, err := store.Active(ctx)
	require.NoError(t, err)
	require.True(t, running, "saving progress must not end the plan")
	assert.Equal(t, StatusCompleted, active.Steps[0].Status)
	assert.Equal(t, StatusRunning, active.Steps[1].Status)
}

func TestFinishRecordsTheOutcome_Integration(t *testing.T) {
	guardIntegration(t)
	ctx := context.Background()
	store := TrackingStore{}

	for name, tc := range map[string]struct {
		runErr error
		status string
	}{
		"success":   {nil, string(tracking.StatusCompleted)},
		"failure":   {errors.New("precompute blew up"), string(tracking.StatusFailed)},
		"cancelled": {context.Canceled, string(tracking.StatusCancelled)},
	} {
		t.Run(name, func(t *testing.T) {
			setupPlanDB(t)
			state := NewState(PlanFull, mustResolve(t, "import"), "2026-08-27T12:00:00Z")
			id, err := store.Create(ctx, PlanFull, state)
			require.NoError(t, err)

			require.NoError(t, store.Finish(ctx, id, state, tc.runErr))

			_, _, running, err := store.Active(ctx)
			require.NoError(t, err)
			assert.False(t, running, "a finished plan is not in flight")

			m, found, err := tracking.LatestForCommand(ctx, PlanCommand)
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, tc.status, string(m.Status),
				"an operator stopping a plan is not the pipeline breaking")
		})
	}
}

// TestPlanRowDoesNotBlockItsOwnSteps_Integration is the reason PlanCommand is not a
// registry step: were it one, the plan's in-progress row would make LaneBusy true and
// block the very steps it exists to run.
func TestPlanRowDoesNotBlockItsOwnSteps_Integration(t *testing.T) {
	guardIntegration(t)
	setupPlanDB(t)
	ctx := context.Background()

	state := NewState(PlanFull, mustResolve(t, "import"), "2026-08-27T12:00:00Z")
	_, err := TrackingStore{}.Create(ctx, PlanFull, state)
	require.NoError(t, err)

	busy, err := tracking.HasInProgressForCommand(ctx, "cricsheet-import")
	require.NoError(t, err)
	assert.False(t, busy, "a running plan must not look like a running import")
}

// TestLatestSurvivesTheBrowserBeingClosed_Integration: the state lives in the
// database, so a page reload — or a browser closed overnight — does not lose the run.
func TestLatestSurvivesTheBrowserBeingClosed_Integration(t *testing.T) {
	guardIntegration(t)
	setupPlanDB(t)
	ctx := context.Background()
	store := TrackingStore{}

	state := NewState(PlanFull, mustResolve(t, "import", "precompute", "export"), "2026-08-27T12:00:00Z")
	id, err := store.Create(ctx, PlanFull, state)
	require.NoError(t, err)
	state.Steps[0].Status = StatusCompleted
	state.Steps[1].Status = StatusFailed
	state.Steps[1].Error = "no rows to precompute"
	require.NoError(t, store.Finish(ctx, id, state, errors.New("precompute failed")))

	// A new reader, with nothing in memory.
	gotID, restored, found, err := store.Latest(ctx)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, id, gotID)

	next, ok := restored.FirstIncomplete()
	require.True(t, ok)
	assert.Equal(t, "precompute", next.StepID, "resume from where it stopped")
	assert.Equal(t, "no rows to precompute", restored.Steps[1].Error)
}
