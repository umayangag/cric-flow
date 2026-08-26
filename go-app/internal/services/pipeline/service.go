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

// StopRun stops the runs in the given lanes: it cancels their job contexts and marks
// their tracking rows CANCELLED. Passing no lanes stops everything, which is what an
// unqualified Stop means.
//
// The two halves take the *same* lane set rather than each deciding for itself. When
// they did not — a cancel func with one slot and a tracking update that took
// inProgress[0] — Stop could kill one job and record a different one as cancelled.
func StopRun(
	ctx context.Context,
	reason string,
	lanes []Lane,
	cancelJob CancelJobFunc,
	cancelMigration CancelMigrationFunc,
) (cancelled int, err error) {
	cancelJob(lanes...)
	return cancelMigration(ctx, reason, CommandsInLanes(lanes))
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
