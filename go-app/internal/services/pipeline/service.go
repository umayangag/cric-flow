// Package pipeline provides pipeline domain operations (stop run, etc.)
// so HTTP handlers can stay thin and delegate to this package.
package pipeline

import "context"

// CancelJobFunc is called to cancel the in-memory job context (e.g. App.CancelCurrentJob).
type CancelJobFunc func()

// CancelMigrationFunc cancels the current in-progress migration in tracking and returns
// whether a run was cancelled and any error (e.g. tracking.CancelInProgressMigration).
type CancelMigrationFunc func(ctx context.Context, reason string) (cancelled bool, err error)

// StopRun cancels the current job and marks the in-progress migration as cancelled.
// It calls cancelJob then cancelMigration(ctx, reason). Returns (cancelled, err) from
// cancelMigration; cancelJob is always invoked.
func StopRun(
	ctx context.Context,
	reason string,
	cancelJob CancelJobFunc,
	cancelMigration CancelMigrationFunc,
) (cancelled bool, err error) {
	cancelJob()
	return cancelMigration(ctx, reason)
}
