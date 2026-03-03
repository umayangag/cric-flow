// Package tracking persists pipeline and migration run state so the API and
// ops UI can show running/completed steps and enforce ordering (e.g. export
// only after precompute).
//
// # Concepts (aligned with data_migrations table)
//
//   - Run ID: Migration.ID — primary key of the run.
//   - Run type: Migration.Command — e.g. "cricsheet-import", "precompute-features",
//     "export-dataset", "train-batting", "ml-auto-tune".
//   - Run state: Migration.Status — one of IN_PROGRESS, COMPLETED, FAILED, CANCELLED.
//
// # State transitions
//
//   - Created: Start (CreateMigration) inserts a row with status IN_PROGRESS.
//   - Completed: UpdateMigrationStatus(..., StatusCompleted, ...) or Tracker.Complete.
//   - Failed: UpdateMigrationStatus(..., StatusFailed, ...) or Tracker.Fail.
//   - Cancelled: UpdateMigrationStatus(..., StatusCancelled, ...), Tracker.Cancel,
//     CancelInProgressMigration (user stop), or CancelStaleInProgressMigrations (startup).
//
// Only one run may be IN_PROGRESS per command in practice; the pipeline layer
// enforces "at most one pipeline step running globally" via HasInProgressForAnyCommand.
//
// # Where runs are created and updated
//
//   - Created: pipeline.RunJob (via Tracker from tracking.Start), CLI export/train steps.
//   - Updated: Tracker.Complete / Fail / Cancel; CaptureExit (defer); CancelInProgressMigration;
//     ReconcileStaleRuns (on API startup).
//   - Surfaced: GetInProgressMigrations, GetRecentMigrations, GetMigrationsPaginated;
//     HasInProgressForCommand, HasCompletedSuccessfullyForCommand — used by ops_status and
//     pipeline step gating.
package tracking
