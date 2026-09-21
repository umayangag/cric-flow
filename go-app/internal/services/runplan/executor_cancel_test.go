package runplan

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// contextHonouringStore refuses any write whose context is already cancelled, which is
// what the real store does: pgx checks the context before it sends the statement and
// returns `context canceled` without touching the database.
//
// The existing fakeStore ignores the context it is handed, so every executor test
// passed while no cancelled plan on the box could record anything at all. This store is
// the missing half.
type contextHonouringStore struct {
	mu sync.Mutex
	// refused counts writes rejected because their context was already cancelled.
	refused    int
	finished   bool
	finalState State
	finalErr   error
}

func (s *contextHonouringStore) write(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		s.refused++
		return err
	}
	return nil
}

func (s *contextHonouringStore) Create(ctx context.Context, _ string, _ State) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.write(ctx); err != nil {
		return 0, err
	}
	return 11, nil
}

func (s *contextHonouringStore) Save(ctx context.Context, _ int, _ State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(ctx)
}

func (s *contextHonouringStore) Finish(ctx context.Context, _ int, state State, runErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.write(ctx); err != nil {
		return err
	}
	s.finished = true
	s.finalState = state
	s.finalErr = runErr
	return nil
}

func (s *contextHonouringStore) Active(context.Context) (int, State, bool, error) {
	return 0, State{}, false, nil
}

func (s *contextHonouringStore) Latest(context.Context) (int, State, bool, error) {
	return 0, State{}, false, nil
}

// errStore answers Active with a failure, and nothing else.
type errStore struct{ activeErr error }

func (e errStore) Create(context.Context, string, State) (int, error) { return 0, nil }
func (e errStore) Save(context.Context, int, State) error             { return nil }
func (e errStore) Finish(context.Context, int, State, error) error    { return nil }
func (e errStore) Active(context.Context) (int, State, bool, error) {
	return 0, State{}, false, e.activeErr
}

func (e errStore) Latest(context.Context) (int, State, bool, error) { return 0, State{}, false, nil }

func fixedNow() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }

// TestRunOutcome_IsRecordedWhateverEndedTheRun is GO-05.
//
// The cancelled case is the defect: `Finish` and the cancellation-path `save` were
// called with the plan's own context, which by then was cancelled, so every write was
// refused with `context canceled` and only logged. The row stayed IN_PROGRESS,
// `TrackingStore.Active` went on reporting a plan in flight, and every later plan was
// answered 409 until the 24-hour stale sweep.
//
// The failed and completed cases are here beside it because § 8.7 asks for the three to
// be distinguishable in the record, not only in a log — and a fix that recorded every
// ending as "cancelled" would satisfy the first case alone.
func TestRunOutcome_IsRecordedWhateverEndedTheRun(t *testing.T) {
	t.Parallel()

	stepFailed := errors.New("ml-service refused the request")

	testCases := []struct {
		name string
		// cancelDuringFirstStep makes the plan's own runner stop the plan, which is what
		// the Stop button does. Forced rather than raced for: the cancellation happens
		// inside the step, so the executor is always at the same place when it lands.
		cancelDuringFirstStep bool
		stepErr               error
		wantOutcome           PlanOutcome
		wantStatuses          []StepStatus
		wantErr               error
	}{
		{
			name:                  "a plan the operator stopped",
			cancelDuringFirstStep: true,
			wantOutcome:           OutcomeCancelled,
			wantStatuses:          []StepStatus{StatusCancelled, StatusCancelled},
			wantErr:               context.Canceled,
		},
		{
			name:         "a plan whose step broke",
			stepErr:      stepFailed,
			wantOutcome:  OutcomeFailed,
			wantStatuses: []StepStatus{StatusFailed, StatusPending},
			wantErr:      stepFailed,
		},
		{
			name:         "a plan that finished",
			wantOutcome:  OutcomeCompleted,
			wantStatuses: []StepStatus{StatusCompleted, StatusCompleted},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &contextHonouringStore{}
			planCtx, cancelPlan := context.WithCancel(context.Background())
			t.Cleanup(cancelPlan)

			executor := &Executor{
				Store: store,
				Now:   fixedNow,
				Run: func(ctx context.Context, _ pipelinesvc.Step) error {
					if tc.cancelDuringFirstStep {
						cancelPlan()
						return ctx.Err()
					}
					return tc.stepErr
				},
			}

			err := executor.Execute(planCtx, PlanRetrainOnly, steps(t, "retrain", "reload"))

			requireRunError(t, err, tc.wantErr)
			require.True(t, store.finished, "the run's outcome must reach the store")
			assert.Zero(t, store.refused, "no bookkeeping write may go through a cancelled context")
			assert.Equal(t, tc.wantOutcome, store.finalState.Outcome)
			assert.Equal(t, tc.wantStatuses, statuses(store.finalState))
			assert.NotEmpty(t, store.finalState.FinishedAt, "a finished run knows when it finished")
		})
	}
}

func requireRunError(t *testing.T, got, want error) {
	t.Helper()
	if want == nil {
		require.NoError(t, got)
		return
	}
	require.ErrorIs(t, got, want)
}

// TestRun_RefusesToStartWhenItCannotTellWhetherAPlanIsRunning is the fail-open half of
// GO-06 as it appears in the executor: `activeErr == nil && running` read a database
// error as "nothing is running" and started a second plan on top of the first.
func TestRun_RefusesToStartWhenItCannotTellWhetherAPlanIsRunning(t *testing.T) {
	t.Parallel()

	unreachable := errors.New("connection refused")
	executor := &Executor{
		Store: errStore{activeErr: unreachable},
		Now:   fixedNow,
		Run: func(context.Context, pipelinesvc.Step) error {
			assert.Fail(t, "no step may run when the exclusivity check could not be answered")
			return nil
		},
	}

	err := executor.Execute(context.Background(), PlanRetrainOnly, steps(t, "retrain"))

	require.ErrorIs(t, err, unreachable)
}
