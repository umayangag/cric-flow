package runplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// PlanCommand is what a plan run is recorded as in data_migrations.
//
// Deliberately not a registry step. A plan is not a stage of the pipeline, it is a
// walk through several — and if it were in the compute lane, the plan's own
// in-progress row would make LaneBusy true and block the very steps it exists to run.
const PlanCommand = "pipeline-plan"

// ErrPlanRunning is returned when a plan is started while one is already in flight.
var ErrPlanRunning = errors.New("a run plan is already in progress")

// StepRunner runs one step to completion and returns when it is done.
//
// Synchronous by design: the executor's whole job is to wait for one step before
// starting the next, and a runner that returned early would turn "sequential" into a
// stampede that CanRunPipelineStep would then refuse at random.
type StepRunner func(ctx context.Context, step pipelinesvc.Step) error

// StepGate answers whether a step may start, and why not when it may not. In
// production this is opsstatus.CanRunPipelineStep — the same rules the single-step
// endpoint applies, rather than a second copy that could disagree with it.
type StepGate func(ctx context.Context, stepID string) (bool, string)

// Store persists plan state. Backed by data_migrations in production.
//
// Implementations receive a copy of the state and may retain it: the executor clones
// before every call, so nothing it does afterwards is visible to what was handed over.
type Store interface {
	// Create records a starting plan and returns its run id.
	Create(ctx context.Context, plan string, state State) (int, error)
	// Save updates the state of an in-flight plan.
	Save(ctx context.Context, id int, state State) error
	// Finish records the terminal state of a plan run.
	Finish(ctx context.Context, id int, state State, runErr error) error
	// Active returns the in-flight plan run, if there is one.
	Active(ctx context.Context) (int, State, bool, error)
	// Latest returns the most recent plan run, in flight or not.
	Latest(ctx context.Context) (int, State, bool, error)
}

// Executor walks a plan's steps in order, stopping at the first failure.
type Executor struct {
	Store Store
	Run   StepRunner
	Gate  StepGate
	// Now is injectable so the timestamps in a plan's state are testable.
	Now func() time.Time
}

// Execute runs a plan from the beginning, to completion or to its first failure.
//
// It returns when the plan is over. Callers that must not block start it in a
// goroutine — the plan's state is persisted as it goes, so nothing is lost if the
// caller stops watching, or if the browser that started it is closed overnight.
func (e *Executor) Execute(ctx context.Context, plan string, steps []pipelinesvc.Step) error {
	return e.run(ctx, plan, steps, nil)
}

// ExecuteSkipping runs a plan, skipping the named steps for the given reasons.
//
// Distinct from Resume, which skips what a previous run finished. This skips what the
// box's current state makes unnecessary — Import does not download an archive the
// dataset directory already holds (consumer plan W6-2). The reason travels with the
// skip because a step that renders as "done" without having run is exactly the
// silent-success failure this codebase keeps meeting.
func (e *Executor) ExecuteSkipping(
	ctx context.Context,
	plan string,
	steps []pipelinesvc.Step,
	skip map[string]string,
) error {
	return e.run(ctx, plan, steps, skip)
}

// Resume continues a plan, skipping the steps that completed in the run being resumed.
//
// "Completed" means completed *in that run*, which is the only sense that makes sense
// here. An earlier version asked whether the step had ever completed successfully —
// so on any box that had run the pipeline before, a fresh `full` plan would skip every
// step and report success having done nothing. That is the silent-success failure this
// codebase keeps meeting, and it is why skipping is now driven by the prior run's own
// state rather than by history at large.
func (e *Executor) Resume(ctx context.Context, plan string, steps []pipelinesvc.Step, prior State) error {
	return e.run(ctx, plan, steps, completedIn(prior))
}

// resumeNote is what a step skipped by a resume records about itself.
const resumeNote = "completed in the run being resumed"

// completedIn returns the steps a prior run finished with, so a resume does not repeat
// them. A step that failed is *not* included: it is where the resume starts.
func completedIn(prior State) map[string]string {
	done := make(map[string]string, len(prior.Steps))
	for _, step := range prior.Steps {
		if step.Status == StatusCompleted || step.Status == StatusSkipped {
			done[step.StepID] = resumeNote
		}
	}
	return done
}

func (e *Executor) run(
	ctx context.Context,
	plan string,
	steps []pipelinesvc.Step,
	skip map[string]string,
) (err error) {
	if _, _, running, activeErr := e.Store.Active(ctx); activeErr == nil && running {
		return ErrPlanRunning
	}

	state := NewState(plan, steps, e.timestamp())
	id, err := e.Store.Create(ctx, plan, state.Clone())
	if err != nil {
		return fmt.Errorf("record plan start: %w", err)
	}

	defer func() {
		state.FinishedAt = e.timestamp()
		if finishErr := e.Store.Finish(ctx, id, state.Clone(), err); finishErr != nil {
			slog.Warn("run plan: recording the outcome failed", slog.Int("id", id), slog.Any("err", finishErr))
		}
	}()

	for i := range state.Steps {
		step := steps[i]
		current := &state.Steps[i]

		// Cancellation stops the plan, not just the step it is on. Checked before
		// each step as well as inside them, so a plan cancelled between steps does
		// not quietly start the next one.
		if ctx.Err() != nil {
			e.markRemaining(&state, i, StatusCancelled)
			e.save(ctx, id, state)
			return ctx.Err()
		}

		if reason, skipped := skip[step.ID]; skipped {
			current.Status = StatusSkipped
			current.Note = reason
			current.FinishedAt = e.timestamp()
			slog.Info("run plan: skipping step",
				slog.String("step", step.ID), slog.String("reason", reason))
			e.save(ctx, id, state)
			continue
		}

		if ok, msg := e.gate(ctx, step.ID); !ok {
			current.Status = StatusFailed
			current.Error = msg
			current.FinishedAt = e.timestamp()
			e.markRemaining(&state, i+1, StatusPending)
			e.save(ctx, id, state)
			return fmt.Errorf("%s cannot run: %s", step.Label, msg)
		}

		current.Status = StatusRunning
		current.StartedAt = e.timestamp()
		e.save(ctx, id, state)

		stepErr := e.Run(ctx, step)
		current.FinishedAt = e.timestamp()

		switch {
		case stepErr == nil:
			current.Status = StatusCompleted
		case errors.Is(stepErr, context.Canceled), errors.Is(stepErr, context.DeadlineExceeded):
			current.Status = StatusCancelled
			current.Error = stepErr.Error()
			e.markRemaining(&state, i+1, StatusCancelled)
			e.save(ctx, id, state)
			return stepErr
		default:
			// Stop on first failure, leaving the rest PENDING so the plan is
			// resumable from here rather than from the top.
			current.Status = StatusFailed
			current.Error = stepErr.Error()
			e.save(ctx, id, state)
			return fmt.Errorf("%s failed: %w", step.Label, stepErr)
		}
		e.save(ctx, id, state)
	}
	return nil
}

// markRemaining sets the status of every step from index onwards that has not already
// reached a terminal state.
func (e *Executor) markRemaining(state *State, from int, status StepStatus) {
	for i := from; i < len(state.Steps); i++ {
		if !state.Steps[i].Status.Terminal() {
			state.Steps[i].Status = status
		}
	}
}

// save persists progress. A failure to record it does not stop the plan: the steps
// are running regardless, and abandoning a working pipeline because a status write
// failed would be the wrong trade.
func (e *Executor) save(ctx context.Context, id int, state State) {
	if err := e.Store.Save(ctx, id, state.Clone()); err != nil {
		slog.Warn("run plan: saving state failed", slog.Int("id", id), slog.Any("err", err))
	}
}

func (e *Executor) gate(ctx context.Context, stepID string) (bool, string) {
	if e.Gate == nil {
		return true, ""
	}
	return e.Gate(ctx, stepID)
}

func (e *Executor) timestamp() string {
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	return now().UTC().Format(time.RFC3339)
}

// StateFromJSON reads plan state out of a data_migrations metadata blob.
func StateFromJSON(raw json.RawMessage) (State, bool) {
	if len(raw) == 0 {
		return State{}, false
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, false
	}
	if state.Plan == "" && len(state.Steps) == 0 {
		return State{}, false
	}
	return state, true
}
