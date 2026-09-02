// Package pipeline provides pipeline domain operations (stop run, etc.)
// so HTTP handlers can stay thin and delegate to this package.
package pipeline

import "context"

// CancelJobFunc cancels the in-memory job contexts for the given lanes and returns
// how many it cancelled. No lanes means every lane (e.g. App.CancelJobsInLanes).
type CancelJobFunc func(lanes ...Lane) int

// CancelMigrationFunc marks the in-flight runs of the given commands CANCELLED in
// tracking and returns how many rows it updated (e.g.
// tracking.CancelInProgressMigrations). An empty command set means every run.
type CancelMigrationFunc func(ctx context.Context, reason string, commands []string) (int, error)

// StopTrainingFunc asks ml-service to stop its training subprocess and returns the steps
// it confirms are stopped (e.g. StopMLTraining).
type StopTrainingFunc func(ctx context.Context) ([]string, error)

// StopOutcome is what a Stop actually achieved, kept apart from what it attempted.
//
// TrainingStopped lists the steps ml-service confirmed it killed; TrainingErr is set when
// it could not be asked or would not say. A caller must not report a stop as done while
// TrainingErr is non-nil — that is D-10 exactly: the console said `{"cancelled": 1}` while
// `ml.xi.retrain` ran on.
type StopOutcome struct {
	Cancelled       int
	TrainingStopped []string
	TrainingErr     error
}

// StopRun stops the runs in the given lanes: it stops the training subprocess on
// ml-service, cancels the local job contexts, and marks the tracking rows CANCELLED.
// Passing no lanes stops everything, which is what an unqualified Stop means.
//
// The three halves take the *same* lane set rather than each deciding for itself. When
// they did not — a cancel func with one slot and a tracking update that took
// inProgress[0] — Stop could kill one job and record a different one as cancelled.
//
// The remote stop goes first, and on purpose: cancelling the local job closes the HTTP
// request the step is waiting on, and once that is gone the run looks finished from here
// whatever is still happening over there. Stop the work, then tidy up the bookkeeping.
func StopRun(
	ctx context.Context,
	reason string,
	lanes []Lane,
	cancelJob CancelJobFunc,
	cancelMigration CancelMigrationFunc,
	stopTraining StopTrainingFunc,
) (StopOutcome, error) {
	outcome := StopOutcome{}
	// Training runs in the compute lane, so a Stop aimed only at the data lane has no
	// business killing it — that is the distinction the lane parameter exists to draw.
	if stopTraining != nil && lanesInclude(lanes, LaneCompute) {
		outcome.TrainingStopped, outcome.TrainingErr = stopTraining(ctx)
	}
	cancelJob(lanes...)
	cancelled, err := cancelMigration(ctx, reason, CommandsInLanes(lanes))
	outcome.Cancelled = cancelled
	return outcome, err
}

// lanesInclude reports whether a lane is in scope. No lanes means every lane.
func lanesInclude(lanes []Lane, want Lane) bool {
	if len(lanes) == 0 {
		return true
	}
	for _, lane := range lanes {
		if lane == want {
			return true
		}
	}
	return false
}

// CommandsInLanes returns the data_migrations commands of every step in the given
// lanes, or nil for "all lanes" so callers can pass the result straight through.
func CommandsInLanes(lanes []Lane) []string {
	if len(lanes) == 0 {
		return nil
	}
	registry := Steps()
	out := make([]string, 0, len(registry.All()))
	for _, lane := range lanes {
		out = append(out, registry.CommandsInLane(lane)...)
	}
	return out
}
