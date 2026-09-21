package runplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// TrackingStore persists plan state in data_migrations.
//
// A plan is a run, and data_migrations is the run-history table: it already has a row
// per run, a status, timestamps and a jsonb column. A dedicated table would have been
// a second place to look for the same thing, and would have needed its own migration,
// its own cleanup and its own answer to "what is running right now?".
type TrackingStore struct{}

// Create records a starting plan and returns its run id, or ErrPlanRunning if one is
// already in flight.
//
// The check and the insert are one transaction under an advisory lock, the same claim a
// pipeline step makes for its lane. `Active` followed by an unguarded insert left the
// same window GO-06 names for steps: two `POST /ops/pipeline/run-plan` inside one round
// trip both read "nothing running" and both started, each overwriting the other's
// bookkeeping. A plan is in no lane, so it contends on its own key rather than a lane's.
func (s TrackingStore) Create(ctx context.Context, plan string, state State) (int, error) {
	tracker, err := tracking.StartExclusive(
		ctx,
		PlanCommand,
		map[string]any{"plan": plan, "steps": stepIDs(state)},
		tracking.AdvisoryKeyFor(PlanCommand),
		[]string{PlanCommand},
	)
	switch {
	case errors.Is(err, tracking.ErrRunConflict):
		return 0, ErrPlanRunning
	case err != nil:
		return 0, fmt.Errorf("claim the plan: %w", err)
	}
	if saveErr := s.Save(ctx, tracker.ID, state); saveErr != nil {
		return tracker.ID, saveErr
	}
	return tracker.ID, nil
}

// Save updates the state of an in-flight plan without touching its status.
func (TrackingStore) Save(ctx context.Context, id int, state State) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode plan state: %w", err)
	}
	return tracking.UpdateMigrationMetadata(ctx, id, encoded)
}

// Finish records the terminal state of a plan run.
//
// A cancelled plan is recorded as CANCELLED rather than FAILED: the operator stopping
// something is not the pipeline breaking, and run history that conflates them makes
// "has this ever failed?" unanswerable.
func (TrackingStore) Finish(ctx context.Context, id int, state State, runErr error) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode plan state: %w", err)
	}

	// The row's status and the state's own Outcome come from one decision (OutcomeFor),
	// so run history and the plan payload cannot disagree about whether a run was
	// stopped or broke.
	switch OutcomeFor(runErr) {
	case OutcomeCompleted:
		return tracking.UpdateMigrationStatus(ctx, id, tracking.StatusCompleted, encoded, "")
	case OutcomeCancelled:
		return tracking.UpdateMigrationStatus(ctx, id, tracking.StatusCancelled, encoded, runErr.Error())
	default:
		return tracking.UpdateMigrationStatus(ctx, id, tracking.StatusFailed, encoded, runErr.Error())
	}
}

// Active returns the in-flight plan run, if there is one.
func (TrackingStore) Active(ctx context.Context) (int, State, bool, error) {
	running, err := tracking.GetInProgressMigrations(ctx)
	if err != nil {
		return 0, State{}, false, err
	}
	for _, m := range running {
		if m.Command != PlanCommand {
			continue
		}
		state, _ := StateFromJSON(m.Metadata)
		return m.ID, state, true, nil
	}
	return 0, State{}, false, nil
}

// Latest returns the most recent plan run, in flight or not.
//
// This is what answers "show me the plan even though I reloaded the page, or started
// it from another tab, or closed the browser overnight" — the state lives in the
// database, so none of those lose it.
func (TrackingStore) Latest(ctx context.Context) (int, State, bool, error) {
	m, found, err := tracking.LatestForCommand(ctx, PlanCommand)
	if err != nil || !found {
		return 0, State{}, false, err
	}
	state, ok := StateFromJSON(m.Metadata)
	if !ok {
		// A plan row whose metadata never landed is still a plan row; reporting it
		// with an empty state beats reporting no plan at all.
		return m.ID, State{}, true, nil
	}
	return m.ID, state, true, nil
}

func stepIDs(state State) []string {
	out := make([]string, 0, len(state.Steps))
	for _, s := range state.Steps {
		out = append(out, s.StepID)
	}
	return out
}
