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

// Create records a starting plan and returns its run id.
func (s TrackingStore) Create(ctx context.Context, plan string, state State) (int, error) {
	args, err := json.Marshal(map[string]any{"plan": plan, "steps": stepIDs(state)})
	if err != nil {
		return 0, fmt.Errorf("encode plan args: %w", err)
	}
	id, err := tracking.CreateMigration(ctx, PlanCommand, args)
	if err != nil {
		return 0, err
	}
	if saveErr := s.Save(ctx, id, state); saveErr != nil {
		return id, saveErr
	}
	return id, nil
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

	switch {
	case runErr == nil:
		return tracking.UpdateMigrationStatus(ctx, id, tracking.StatusCompleted, encoded, "")
	case errors.Is(runErr, context.Canceled), errors.Is(runErr, context.DeadlineExceeded):
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
