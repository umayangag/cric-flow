// Package tracking persists pipeline and migration run state so the API and
// ops UI can show running/completed steps and enforce ordering (e.g. reload
// only after retrain).
//
// # Concepts (aligned with data_migrations table)
//
//   - Run ID: Migration.ID — primary key of the run.
//   - Run type: Migration.Command — e.g. "cricsheet-import", "xi-retrain", "xi-evaluate",
//     "xi-reload", "dataset-fetch", "dataset-extract".
//   - Run state: Migration.Status — one of IN_PROGRESS, COMPLETED, FAILED, CANCELLED.
//
// # State transitions
//
//   - Created: StartExclusive inserts a row with status IN_PROGRESS, inside one
//     transaction that first takes a Postgres advisory lock on the resource the run
//     contends for and re-reads whether anything already holds it. The check and the
//     insert are one thing on purpose (GO-06): as two statements, two requests in the
//     same round trip both read "free" and both started.
//   - Completed: UpdateMigrationStatus(..., StatusCompleted, ...) or Tracker.Complete.
//   - Failed: UpdateMigrationStatus(..., StatusFailed, ...) or Tracker.Fail.
//   - Cancelled: UpdateMigrationStatus(..., StatusCancelled, ...), Tracker.Cancel,
//     CancelInProgressMigration (user stop), or CancelStaleInProgressMigrations (startup).
//
// Only one run may be IN_PROGRESS per lane; StartExclusive is what enforces it, and
// HasInProgressForAnyCommand is the advisory pre-flight handlers use to answer 409
// before starting anything.
//
// # Where runs are created and updated
//
//   - Created: pipeline.RunJob and runplan.TrackingStore.Create, both via StartExclusive.
//   - Updated: Tracker.Complete / Fail / Cancel; CaptureExit (defer); CancelInProgressMigration;
//     CancelStaleRuns (on API startup).
//   - Surfaced: GetInProgressMigrations, GetRecentMigrations, GetMigrationsPaginated;
//     HasInProgressForCommand, LastRunSucceededForCommand — used by ops_status and
//     pipeline step gating.
package tracking
