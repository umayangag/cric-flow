package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func CreateMigration(ctx context.Context, command string, args json.RawMessage) (int, error) {
	if db.Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	var id int
	err := db.Pool.QueryRow(ctx, `
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
	if db.Pool == nil {
		return errors.New("db pool not initialized")
	}

	// Prepare optional error message
	var errMsgPtr *string
	if errorMsg != "" {
		errMsgPtr = &errorMsg
	}

	_, err := db.Pool.Exec(ctx, `
		UPDATE data_migrations
		SET status = $2, completed_at = NOW(), metadata = $3, error_message = $4
		WHERE id = $1
	`, id, status, metadata, errMsgPtr)
	return err
}

func GetRecentMigrations(ctx context.Context, limit int) ([]Migration, error) {
	if db.Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := db.Pool.Query(ctx, `
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
	if db.Pool == nil {
		return nil, 0, errors.New("db pool not initialized")
	}

	// Get total count
	var total int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM data_migrations`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.Pool.Query(ctx, `
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
