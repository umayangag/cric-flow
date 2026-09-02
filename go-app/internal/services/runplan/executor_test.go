package runplan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// fakeStore records what the executor persisted, in order.
type fakeStore struct {
	id        int
	saves     []State
	final     State
	finalErr  error
	active    bool
	createErr error
	saveErr   error
}

func (f *fakeStore) Create(_ context.Context, _ string, state State) (int, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.id = 7
	f.saves = append(f.saves, state)
	return f.id, nil
}

func (f *fakeStore) Save(_ context.Context, _ int, state State) error {
	f.saves = append(f.saves, state)
	return f.saveErr
}

func (f *fakeStore) Finish(_ context.Context, _ int, state State, runErr error) error {
	f.final = state
	f.finalErr = runErr
	return nil
}

func (f *fakeStore) Active(context.Context) (int, State, bool, error) {
	return 0, State{}, f.active, nil
}

func (f *fakeStore) Latest(context.Context) (int, State, bool, error) {
	return f.id, f.final, f.id != 0, nil
}

func steps(t *testing.T, ids ...string) []pipelinesvc.Step {
	t.Helper()
	out, err := Resolve("", ids)
	require.NoError(t, err)
	return out
}

// executorFor builds an executor whose steps all succeed unless the runner says
// otherwise.
func executorFor(store *fakeStore, run StepRunner) *Executor {
	return &Executor{
		Store: store,
		Run:   run,
		Now:   func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) },
	}
}

func statuses(state State) []StepStatus {
	out := make([]StepStatus, 0, len(state.Steps))
	for _, s := range state.Steps {
		out = append(out, s.Status)
	}
	return out
}

func TestExecute_RunsEveryStepInOrder(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		return nil
	})

	plan := steps(t, "import", "retrain", "reload")
	require.NoError(t, exec.Execute(context.Background(), PlanFull, plan))

	assert.Equal(t, []string{"import", "retrain", "reload"}, ran)
	assert.Equal(t, []StepStatus{StatusCompleted, StatusCompleted, StatusCompleted}, statuses(store.final))
	assert.NoError(t, store.finalErr)
	assert.NotEmpty(t, store.final.FinishedAt)
}

// TestExecute_StopsAtTheFirstFailure is the plan's core promise. The remaining steps
// stay PENDING rather than being marked failed, which is what makes the plan resumable
// from where it stopped rather than from the top.
func TestExecute_StopsAtTheFirstFailure(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		if step.ID == "retrain" {
			return errors.New("nothing to retrain")
		}
		return nil
	})

	plan := steps(t, "import", "retrain", "reload")
	err := exec.Execute(context.Background(), PlanFull, plan)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to retrain")
	assert.Equal(t, []string{"import", "retrain"}, ran, "reload must not run after a failure")
	assert.Equal(t, []StepStatus{StatusCompleted, StatusFailed, StatusPending}, statuses(store.final))

	failed, ok := store.final.Failed()
	require.True(t, ok)
	assert.Equal(t, "retrain", failed.StepID)
	assert.Contains(t, failed.Error, "nothing to retrain")

	next, ok := store.final.FirstIncomplete()
	require.True(t, ok)
	assert.Equal(t, "retrain", next.StepID, "resume starts at the step that failed")
}

// TestExecute_CancellationStopsThePlanNotJustTheStep: cancelling only the current step
// would stop it and then start the next one, which is not what Stop means.
func TestExecute_CancellationStopsThePlanNotJustTheStep(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		if step.ID == "import" {
			cancel()
			return context.Canceled
		}
		return nil
	})

	plan := steps(t, "import", "retrain", "reload")
	err := exec.Execute(ctx, PlanFull, plan)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"import"}, ran)
	assert.Equal(t, []StepStatus{StatusCancelled, StatusCancelled, StatusCancelled}, statuses(store.final))
}

// TestExecute_CancelledBetweenStepsDoesNotStartTheNext covers the window the
// per-step check exists for.
func TestExecute_CancelledBetweenStepsDoesNotStartTheNext(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		// Succeeds, but the plan is cancelled by the time it returns.
		cancel()
		return nil
	})

	err := exec.Execute(ctx, PlanFull, steps(t, "import", "retrain"))

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"import"}, ran)
	assert.Equal(t, []StepStatus{StatusCompleted, StatusCancelled}, statuses(store.final))
}

// TestExecute_RefusesAStepTheGateRejects: the plan uses the same ordering rules as
// the single-step endpoint rather than its own copy.
func TestExecute_RefusesAStepTheGateRejects(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		return nil
	})
	exec.Gate = func(_ context.Context, stepID string) (bool, string) {
		if stepID == "retrain" {
			return false, "complete the previous step (Import) first"
		}
		return true, ""
	}

	err := exec.Execute(context.Background(), PlanFull, steps(t, "import", "retrain", "reload"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "complete the previous step")
	assert.Equal(t, []string{"import"}, ran)
	assert.Equal(t, []StepStatus{StatusCompleted, StatusFailed, StatusPending}, statuses(store.final))
}

// TestResume_SkipsWhatTheLastRunFinished is what makes resuming cheap: re-running an
// eleven-step pipeline because step nine failed is how an operator learns not to use
// the button.
func TestResume_SkipsWhatTheLastRunFinished(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		return nil
	})

	prior := State{Plan: PlanFull, Steps: []StepState{
		{StepID: "import", Status: StatusCompleted},
		{StepID: "retrain", Status: StatusFailed},
	}}
	require.NoError(t, exec.Resume(context.Background(), PlanFull, steps(t, "import", "retrain"), prior))

	assert.Equal(t, []string{"retrain"}, ran)
	assert.Equal(t, []StepStatus{StatusSkipped, StatusCompleted}, statuses(store.final))
}

// TestResume_RerunsTheStepThatFailed: the failed step is where the resume starts, not
// something to skip past.
func TestResume_RerunsTheStepThatFailed(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		return nil
	})

	prior := State{Plan: PlanFull, Steps: []StepState{
		{StepID: "import", Status: StatusCompleted},
		{StepID: "retrain", Status: StatusFailed},
		{StepID: "reload", Status: StatusPending},
	}}
	require.NoError(t, exec.Resume(context.Background(), PlanFull, steps(t, "import", "retrain", "reload"), prior))

	assert.Equal(t, []string{"retrain", "reload"}, ran)
}

// TestExecute_RunsEveryStepEvenOnABoxThatHasRunThemBefore is the regression guard for
// a bug that would have been catastrophic and silent.
//
// An earlier version asked tracking whether each step had *ever* completed
// successfully, and skipped it if so. On any box that had run the pipeline once, a
// fresh `full` plan would therefore skip every step and report success having done
// nothing — the exact silent-success failure this codebase keeps meeting. Skipping is
// now driven by the prior run's own state, and a fresh Execute has no prior run.
func TestExecute_RunsEveryStepEvenOnABoxThatHasRunThemBefore(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		return nil
	})

	// A previous run of this plan completed everything. A fresh Execute must ignore it.
	store.final = State{Plan: PlanFull, Steps: []StepState{
		{StepID: "import", Status: StatusCompleted},
		{StepID: "retrain", Status: StatusCompleted},
	}}

	require.NoError(t, exec.Execute(context.Background(), PlanFull, steps(t, "import", "retrain")))

	assert.Equal(t, []string{"import", "retrain"}, ran,
		"a fresh plan runs every step; skipping history is how a plan silently does nothing")
	assert.Equal(t, []StepStatus{StatusCompleted, StatusCompleted}, statuses(store.final))
}

func TestExecute_RefusesToStartWhenAPlanIsAlreadyRunning(t *testing.T) {
	t.Parallel()
	store := &fakeStore{active: true}
	ran := 0
	exec := executorFor(store, func(context.Context, pipelinesvc.Step) error {
		ran++
		return nil
	})

	err := exec.Execute(context.Background(), PlanFull, steps(t, "import"))
	require.ErrorIs(t, err, ErrPlanRunning)
	assert.Zero(t, ran)
}

func TestExecute_FailsWhenTheStartCannotBeRecorded(t *testing.T) {
	t.Parallel()
	store := &fakeStore{createErr: errors.New("db down")}
	ran := 0
	exec := executorFor(store, func(context.Context, pipelinesvc.Step) error {
		ran++
		return nil
	})

	err := exec.Execute(context.Background(), PlanFull, steps(t, "import"))
	require.Error(t, err)
	assert.Zero(t, ran, "a plan nobody can see the state of must not run invisibly")
}

// TestExecute_KeepsGoingWhenProgressCannotBeSaved: the steps are running regardless,
// and abandoning a working pipeline because a status write failed is the wrong trade.
func TestExecute_KeepsGoingWhenProgressCannotBeSaved(t *testing.T) {
	t.Parallel()
	store := &fakeStore{saveErr: errors.New("db hiccup")}
	var ran []string
	exec := executorFor(store, func(_ context.Context, step pipelinesvc.Step) error {
		ran = append(ran, step.ID)
		return nil
	})

	require.NoError(t, exec.Execute(context.Background(), PlanFull, steps(t, "import", "retrain")))
	assert.Equal(t, []string{"import", "retrain"}, ran)
}

// TestExecute_PersistsProgressAsItGoes is what survives a page reload, or a browser
// closed overnight: the state is written before and after each step, not only at the
// end.
func TestExecute_PersistsProgressAsItGoes(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	exec := executorFor(store, func(context.Context, pipelinesvc.Step) error { return nil })

	require.NoError(t, exec.Execute(context.Background(), PlanFull, steps(t, "import", "retrain")))

	sawRunning := false
	for _, saved := range store.saves {
		for _, step := range saved.Steps {
			if step.Status == StatusRunning {
				sawRunning = true
			}
		}
	}
	assert.True(t, sawRunning, "a reader mid-run must be able to see which step is in flight")
	assert.GreaterOrEqual(t, len(store.saves), 4)
}
