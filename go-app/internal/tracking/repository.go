package tracking

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func CreateMigration(ctx context.Context, command string, args json.RawMessage) (int, error) {
	var id int
	err := db.QueryRow(ctx, `
		INSERT INTO data_migrations (command, args, status, started_at)
		VALUES ($1, $2, $3, NOW())
		RETURNING id
	`, command, args, StatusInProgress).Scan(&id)
	return id, err
}

func UpdateMigrationStatus(
	ctx context.Context,
	id int,
	status MigrationStatus,
	metadata json.RawMessage,
	errorMsg string,
) error {
	// Prepare optional error message
	var errMsgPtr *string
	if errorMsg != "" {
		errMsgPtr = &errorMsg
	}

	return db.Exec(ctx, `
		UPDATE data_migrations
		SET status = $2, completed_at = NOW(), metadata = $3, error_message = $4
		WHERE id = $1
	`, id, status, metadata, errMsgPtr)
}

// UpdateMigrationMetadata replaces a run's metadata without touching its status.
//
// The status-changing update sets completed_at, which is right for a run that has
// ended and wrong for one still going. A run plan writes its progress as it walks its
// steps (ops plan R-1), and doing that through UpdateMigrationStatus would mark the
// plan finished on its first step.
func UpdateMigrationMetadata(ctx context.Context, id int, metadata json.RawMessage) error {
	if !db.Available() {
		return nil
	}
	return db.Exec(ctx, `
		UPDATE data_migrations
		SET metadata = $2
		WHERE id = $1
	`, id, metadata)
}

// LatestForCommand returns the most recent run of a command, or ok=false when there
// is none.
func LatestForCommand(ctx context.Context, command string) (Migration, bool, error) {
	if !db.Available() {
		return Migration{}, false, nil
	}
	rows, err := db.Query(ctx, `
		SELECT id, command, args, started_at, completed_at, status, metadata, error_message
		FROM data_migrations
		WHERE command = $1
		ORDER BY started_at DESC
		LIMIT 1
	`, command)
	if err != nil {
		return Migration{}, false, err
	}
	defer rows.Close()

	found, err := scanMigrations(rows)
	if err != nil || len(found) == 0 {
		return Migration{}, false, err
	}
	return found[0], true, nil
}

// HasInProgressForCommand returns true if there is at least one row in data_migrations
// for the given command with status IN_PROGRESS. Used by /ops/status pipeline section.
// When the db pool is not initialized (e.g. disconnected), returns (false, nil).
func HasInProgressForCommand(ctx context.Context, command string) (bool, error) {
	if !db.Available() {
		return false, nil
	}
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM data_migrations
			WHERE command = $1 AND status = $2
		)
	`, command, StatusInProgress).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// CancelInProgressMigrations sets every IN_PROGRESS run whose command is in the given
// set to CANCELLED, and returns how many rows it updated. An empty set cancels every
// in-flight run.
//
// It used to cancel inProgress[0] and call that "the" run — the same single-slot
// assumption the SSE stream carried (ops plan F-2) and for the same reason: one global
// lock made it true. It stopped being true when acquisition got its own lane, and the
// failure was quiet in the worst way: Stop cancelled the context of one job and marked
// a different job's row CANCELLED, leaving one run killed but recorded as running and
// another recorded as cancelled but still going.
func CancelInProgressMigrations(ctx context.Context, reason string, commands []string) (int, error) {
	if !db.Available() {
		return 0, nil
	}
	inProgress, err := GetInProgressMigrations(ctx)
	if err != nil || len(inProgress) == 0 {
		return 0, err
	}

	wanted := make(map[string]bool, len(commands))
	for _, c := range commands {
		wanted[c] = true
	}

	cancelled := 0
	for _, m := range inProgress {
		if len(wanted) > 0 && !wanted[m.Command] {
			continue
		}
		if err := UpdateMigrationStatus(ctx, m.ID, StatusCancelled, nil, reason); err != nil {
			return cancelled, err
		}
		cancelled++
	}
	return cancelled, nil
}

// CancelStaleInProgressMigrations sets IN_PROGRESS rows to CANCELLED only when started_at
// is older than the given threshold. Use on server startup so that runs interrupted by
// this instance's restart/crash are cleaned up, without cancelling runs started recently
// by another instance (e.g. B running a pipeline while A restarts).
// If staleOlderThan <= 0, no rows are cancelled. Returns the number of rows updated.
func CancelStaleInProgressMigrations(ctx context.Context, reason string, staleOlderThan time.Duration) (int, error) {
	if !db.Available() || staleOlderThan < time.Second {
		return 0, nil
	}
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	staleSeconds := int64(staleOlderThan.Seconds())
	var n int
	err := db.QueryRow(ctx, `
		WITH updated AS (
			UPDATE data_migrations
			SET status = $1, completed_at = NOW(), error_message = $2
			WHERE status = $3 AND started_at < NOW() - ($4::bigint * interval '1 second')
			RETURNING 1
		)
		SELECT COUNT(*)::int FROM updated
	`, StatusCancelled, reasonPtr, StatusInProgress, staleSeconds).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// CancelStaleRuns cancels IN_PROGRESS runs older than staleOlderThan (e.g. on API startup
// so runs interrupted by restart/crash are marked CANCELLED). Logs how many were cancelled.
// Use when db is already connected. Returns the number of runs cancelled and any error.
func CancelStaleRuns(ctx context.Context, reason string, staleOlderThan time.Duration) (int, error) {
	n, err := CancelStaleInProgressMigrations(ctx, reason, staleOlderThan)
	if err != nil {
		return n, err
	}
	if n > 0 {
		slog.Info(
			"cancelled stale in-progress pipeline runs",
			slog.Int("count", n),
			slog.Duration("stale_older_than", staleOlderThan),
		)
	}
	return n, nil
}

// LastRunSucceededForCommand reports whether the most recent run of a command
// completed. Used to gate pipeline steps: the next step is only runnable after the
// previous one succeeded. When the db pool is nil, returns (false, nil).
//
// The most recent run, not any run ever. "Has this command ever completed?" is a
// question about the box's history, not about the data on it: a step that succeeded
// once and has failed or been cancelled every time since went on showing a green tick,
// and went on letting the steps after it run against output that was never rebuilt.
func LastRunSucceededForCommand(ctx context.Context, command string) (bool, error) {
	if !db.Available() || command == "" {
		return false, nil
	}
	latest, found, err := LatestForCommand(ctx, command)
	if err != nil || !found {
		return false, err
	}
	return latest.Status == StatusCompleted, nil
}

// GetLastCompletedAtForCommand returns the completed_at of the most recent COMPLETED row
// for the given command. Used so the precompute section can show "complete" from persisted
// tracking (e.g. after API restart or when precompute was run via CLI). When db pool is nil
// or no completed run exists, returns (nil, nil).
func GetLastCompletedAtForCommand(ctx context.Context, command string) (*time.Time, error) {
	if !db.Available() || command == "" {
		return nil, nil
	}
	var completedAt *time.Time
	err := db.QueryRow(ctx, `
		SELECT completed_at FROM data_migrations
		WHERE command = $1 AND status = $2
		ORDER BY completed_at DESC NULLS LAST
		LIMIT 1
	`, command, StatusCompleted).Scan(&completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if completedAt == nil {
		return nil, nil
	}
	return completedAt, nil
}

// HasInProgressForAnyCommand returns true if there is at least one row with status IN_PROGRESS
// and command in the given list. Used to enforce singleton pipeline: only one pipeline step
// may run across the whole system. When db pool is nil, returns (false, nil).
func HasInProgressForAnyCommand(ctx context.Context, commands []string) (bool, error) {
	if !db.Available() || len(commands) == 0 {
		return false, nil
	}
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM data_migrations
			WHERE status = $1 AND command = ANY($2::text[])
		)
	`, StatusInProgress, commands).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// GetInProgressMigrationIDForCommand returns the ID of the most recent IN_PROGRESS migration
// for the given command, or 0 if none. Used when inserting ml_tuned_params to link to the
// current auto_tune run.
func GetInProgressMigrationIDForCommand(ctx context.Context, command string) (int, error) {
	if !db.Available() || command == "" {
		return 0, nil
	}
	var id int
	err := db.QueryRow(ctx, `
		SELECT id FROM data_migrations
		WHERE command = $1 AND status = $2
		ORDER BY started_at DESC
		LIMIT 1
	`, command, StatusInProgress).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

// GetInProgressMigrations returns all rows with status IN_PROGRESS, ordered by started_at DESC.
// Used by /ops/status pipeline overview. When db is not available, returns (nil, nil).
func GetInProgressMigrations(ctx context.Context) ([]Migration, error) {
	if !db.Available() {
		return nil, nil
	}
	rows, err := db.Query(ctx, `
		SELECT id, command, args, started_at, completed_at, status, metadata, error_message
		FROM data_migrations
		WHERE status = $1
		ORDER BY started_at DESC
	`, StatusInProgress)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMigrations(rows)
}

// InProgressByCommand returns a set of commands that currently have an IN_PROGRESS run.
// Call this once when building pipeline/ops status to avoid N separate HasInProgressForCommand calls.
func InProgressByCommand(ctx context.Context) (map[string]bool, error) {
	list, err := GetInProgressMigrations(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(list))
	for _, m := range list {
		out[m.Command] = true
	}
	return out, nil
}

func scanMigrations(rows db.Rows) ([]Migration, error) {
	var migrations []Migration
	for rows.Next() {
		var m Migration
		var args []byte
		var metadata []byte
		var errMsg *string
		var completedAt *time.Time
		if err := rows.Scan(&m.ID, &m.Command, &args, &m.StartedAt, &completedAt, &m.Status, &metadata, &errMsg); err != nil {
			return nil, err
		}
		m.Args = args
		m.Metadata = metadata
		m.CompletedAt = completedAt
		if errMsg != nil {
			m.ErrorMessage = *errMsg
		}
		migrations = append(migrations, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return migrations, nil
}

func GetRecentMigrations(ctx context.Context, limit int) ([]Migration, error) {
	if !db.Available() {
		return nil, nil
	}
	rows, err := db.Query(ctx, `
		SELECT id, command, args, started_at, completed_at, status, metadata, error_message
		FROM data_migrations
		ORDER BY started_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMigrations(rows)
}

func GetMigrationsPaginated(ctx context.Context, limit, offset int) ([]Migration, int, error) {
	if !db.Available() {
		return nil, 0, nil
	}
	var total int
	err := db.QueryRow(ctx, `SELECT COUNT(*) FROM data_migrations`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx, `
		SELECT id, command, args, started_at, completed_at, status, metadata, error_message
		FROM data_migrations
		ORDER BY started_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	migrations := []Migration{}
	for rows.Next() {
		var m Migration
		var args []byte
		var metadata []byte
		var errMsg *string
		var completedAt *time.Time

		if err := rows.Scan(&m.ID, &m.Command, &args, &m.StartedAt, &completedAt, &m.Status, &metadata, &errMsg); err != nil {
			return nil, 0, err
		}
		m.Args = args
		m.Metadata = metadata
		m.CompletedAt = completedAt
		if errMsg != nil {
			m.ErrorMessage = *errMsg
		}
		migrations = append(migrations, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return migrations, total, nil
}
