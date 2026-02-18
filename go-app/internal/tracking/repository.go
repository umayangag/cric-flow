package tracking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
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

// HasInProgressForCommand returns true if there is at least one row in data_migrations
// for the given command with status IN_PROGRESS. Used by /ops/status pipeline section.
// When the db pool is not initialized (e.g. disconnected), returns (false, nil).
func HasInProgressForCommand(ctx context.Context, command string) (bool, error) {
	if db.Pool == nil {
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

// CancelStaleInProgressMigrations sets IN_PROGRESS rows to CANCELLED only when started_at
// is older than the given threshold. Use on server startup so that runs interrupted by
// this instance's restart/crash are cleaned up, without cancelling runs started recently
// by another instance (e.g. B running a pipeline while A restarts).
// If staleOlderThan <= 0, no rows are cancelled. Returns the number of rows updated.
func CancelStaleInProgressMigrations(ctx context.Context, reason string, staleOlderThan time.Duration) (int, error) {
	if db.Pool == nil || staleOlderThan < time.Second {
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

// HasCompletedSuccessfullyForCommand returns true if there is at least one row for the given
// command with status COMPLETED. Used to gate pipeline steps: next step is only runnable
// after the previous completed successfully. When db pool is nil, returns (false, nil).
func HasCompletedSuccessfullyForCommand(ctx context.Context, command string) (bool, error) {
	if db.Pool == nil || command == "" {
		return false, nil
	}
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM data_migrations
			WHERE command = $1 AND status = $2
		)
	`, command, StatusCompleted).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// HasInProgressForAnyCommand returns true if there is at least one row with status IN_PROGRESS
// and command in the given list. Used to enforce singleton pipeline: only one pipeline step
// may run across the whole system. When db pool is nil, returns (false, nil).
func HasInProgressForAnyCommand(ctx context.Context, commands []string) (bool, error) {
	if db.Pool == nil || len(commands) == 0 {
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

// GetInProgressMigrations returns all rows with status IN_PROGRESS, ordered by started_at DESC.
// Used by /ops/status pipeline overview. When db pool is nil, returns (nil, nil).
func GetInProgressMigrations(ctx context.Context) ([]Migration, error) {
	if db.Pool == nil {
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
	if db.Pool == nil {
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
	if db.Pool == nil {
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
