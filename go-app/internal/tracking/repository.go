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

func GetRecentMigrations(ctx context.Context, limit int) ([]Migration, error) {
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

	migrations := []Migration{}
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

func GetMigrationsPaginated(ctx context.Context, limit, offset int) ([]Migration, int, error) {
	// Get total count (approximate for performance on large tables)
	var total int
	err := db.QueryRow(ctx, `
		SELECT reltuples::bigint 
		FROM pg_class 
		WHERE relname = 'data_migrations' 
		  AND relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'public')
	`).Scan(&total)
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
