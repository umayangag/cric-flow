package runplan

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// What a stopped plan actually leaves behind in data_migrations, against a real
// database (GO-05). The unit tests prove the executor calls Finish with a live context;
// only this proves the row an operator meets afterwards.
//
//	make -C go-app test-db

// planRow reads the one thing the operator's next attempt depends on: the row's status.
func planRow(t *testing.T) (status, errorMessage string, metadata []byte) {
	t.Helper()
	row := db.QueryRow(context.Background(), `
		SELECT status, COALESCE(error_message, ''), COALESCE(metadata::text, '')
		FROM data_migrations WHERE command = $1 ORDER BY id DESC LIMIT 1
	`, PlanCommand)
	var meta string
	require.NoError(t, row.Scan(&status, &errorMessage, &meta))
	return status, errorMessage, []byte(meta)
}

// executorStoppedBy runs a two-step plan whose first step stops the plan by cancelling
// the context it is handed, and returns the error the plan ended with.
//
// `stop` is what does the stopping: the operator's Stop cancels the plan's own context,
// and SIGTERM cancels the job context the plan's context descends from. Both are
// exercised because the two reach the executor by different routes and only one of them
// also has to beat the pool closing.
func executorStoppedBy(t *testing.T, stop func(cancelPlan, cancelJob context.CancelFunc)) error {
	t.Helper()
	jobCtx, cancelJob := context.WithCancel(context.Background())
	t.Cleanup(cancelJob)
	planCtx, cancelPlan := context.WithCancel(jobCtx)
	t.Cleanup(cancelPlan)

	executor := &Executor{
		Store: TrackingStore{},
		Run: func(ctx context.Context, _ pipelinesvc.Step) error {
			stop(cancelPlan, cancelJob)
			return ctx.Err()
		},
	}
	return executor.Execute(planCtx, PlanRetrainOnly, mustResolve(t, "retrain", "reload"))
}

func TestStoppedPlanLeavesACancelledRow_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	testCases := []struct {
		name string
		stop func(cancelPlan, cancelJob context.CancelFunc)
	}{
		{
			name: "the operator presses Stop",
			stop: func(cancelPlan, _ context.CancelFunc) { cancelPlan() },
		},
		{
			name: "the process receives SIGTERM",
			stop: func(_, cancelJob context.CancelFunc) { cancelJob() },
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			setupPlanDB(t)
			ctx := context.Background()

			err := executorStoppedBy(t, tc.stop)
			require.ErrorIs(t, err, context.Canceled)

			status, errorMessage, metadata := planRow(t)
			assert.Equal(t, string(tracking.StatusCancelled), status,
				"the row must say the run was stopped, not that it is still going")
			assert.Contains(t, errorMessage, "context canceled")

			restored, ok := StateFromJSON(metadata)
			require.True(t, ok, "the step-by-step state must survive the stop")
			assert.Equal(t, OutcomeCancelled, restored.Outcome)
			assert.NotEmpty(t, restored.FinishedAt)

			// This is the operator-visible half: `Active` is what answers 409.
			_, _, running, activeErr := TrackingStore{}.Active(ctx)
			require.NoError(t, activeErr)
			assert.False(t, running, "a stopped plan must not read as still running")

			// And so the next plan starts straight away rather than in 24 hours.
			second := &Executor{
				Store: TrackingStore{},
				Run:   func(context.Context, pipelinesvc.Step) error { return nil },
			}
			require.NoError(t, second.Execute(ctx, PlanRetrainOnly, mustResolve(t, "retrain")))
		})
	}
}

// TestCancellingTheRowKeepsThePlanState_Integration covers the Stop endpoint's other
// half: /ops/pipeline/stop marks the in-flight rows CANCELLED through
// tracking.CancelInProgressMigrations, which passes no metadata. That used to overwrite
// the plan's own state with NULL, so a stopped plan came back from the database with
// nothing to show for itself.
func TestCancellingTheRowKeepsThePlanState_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	setupPlanDB(t)
	ctx := context.Background()

	state := NewState(PlanRetrainOnly, mustResolve(t, "retrain", "reload"), "2026-09-21T12:00:00Z")
	state.Steps[0].Status = StatusRunning
	_, err := TrackingStore{}.Create(ctx, PlanRetrainOnly, state)
	require.NoError(t, err)

	cancelled, err := tracking.CancelInProgressMigrations(ctx, "cancelled by user", []string{PlanCommand})
	require.NoError(t, err)
	assert.Equal(t, 1, cancelled, "a lane-scoped Stop must reach the plan's own row")

	status, _, metadata := planRow(t)
	assert.Equal(t, string(tracking.StatusCancelled), status)
	restored, ok := StateFromJSON(metadata)
	require.True(t, ok, "cancelling the row must not erase the plan it describes")
	assert.Equal(t, StatusRunning, restored.Steps[0].Status)
}

// TestConcurrentPlanStartsStartExactlyOnePlan_Integration is GO-06's shape applied to
// the plan's own claim: `Active` followed by an unguarded insert let two
// `POST /ops/pipeline/run-plan` inside one round trip both start, each overwriting the
// other's bookkeeping. Create now claims under the same advisory lock a step's lane
// uses, so the losers are refused rather than admitted.
func TestConcurrentPlanStartsStartExactlyOnePlan_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	setupPlanDB(t)
	ctx := context.Background()

	const claimants = 8
	results := make(chan error, claimants)
	state := NewState(PlanRetrainOnly, mustResolve(t, "retrain"), "2026-09-21T12:00:00Z")
	for i := 0; i < claimants; i++ {
		go func() {
			_, err := TrackingStore{}.Create(ctx, PlanRetrainOnly, state)
			results <- err
		}()
	}

	claimed, refused := 0, 0
	for i := 0; i < claimants; i++ {
		countClaim(t, <-results, &claimed, &refused)
	}
	assert.Equal(t, 1, claimed, "exactly one plan may be started")
	assert.Equal(t, claimants-1, refused)

	var rows int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM data_migrations WHERE command = $1
	`, PlanCommand).Scan(&rows))
	assert.Equal(t, 1, rows, "exactly one plan row may exist")
}

func countClaim(t *testing.T, err error, claimed, refused *int) {
	t.Helper()
	if err == nil {
		*claimed++
		return
	}
	require.ErrorIs(t, err, ErrPlanRunning, "a refused plan says so rather than failing oddly")
	*refused++
}
