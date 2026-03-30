package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// InsertMLTunedParams inserts a row into ml_tuned_params. params and metrics are stored as JSONB.
// metrics may be nil; a nil value is stored as NULL.
// dataMigrationID links to data_migrations (e.g. the IN_PROGRESS ml-auto-tune run); pass 0 to omit.
func InsertMLTunedParams(
	ctx context.Context,
	model, format string,
	params json.RawMessage,
	metrics json.RawMessage,
	dataMigrationID int,
) error {
	if dataMigrationID > 0 {
		_, err := Pool.Exec(ctx, `
			INSERT INTO ml_tuned_params (model, format, params, metrics, data_migration_id)
			VALUES ($1, $2, $3, $4, $5)
		`, model, format, params, metrics, dataMigrationID)
		return err
	}
	_, err := Pool.Exec(ctx, `
		INSERT INTO ml_tuned_params (model, format, params, metrics)
		VALUES ($1, $2, $3, $4)
	`, model, format, params, metrics)
	return err
}

// MLTunedParamsRow is the row returned by GetLatestMLTunedParams.
type MLTunedParamsRow struct {
	Params    json.RawMessage
	Metrics   json.RawMessage
	CreatedAt string
}

// GetLatestMLTunedParams returns the most recent params for the given model and format.
// format may be empty to mean "any"; the query returns the latest row for (model, format).
// Returns (nil, nil) when no row exists.
func GetLatestMLTunedParams(ctx context.Context, model, format string) (*MLTunedParamsRow, error) {
	var params json.RawMessage
	var metrics json.RawMessage
	var createdAt string
	err := Pool.QueryRow(ctx, `
		SELECT params, metrics, created_at
		FROM ml_tuned_params
		WHERE model = $1 AND (format = $2 OR ($2 = '' AND format = ''))
		ORDER BY created_at DESC
		LIMIT 1
	`, model, format).Scan(&params, &metrics, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &MLTunedParamsRow{Params: params, Metrics: metrics, CreatedAt: createdAt}, nil
}

// MLTunedParamsEntry holds one row from ml_tuned_params (model, format, created_at, metrics).
// ListLatestMLTunedParams returns the latest such row per (model, format).
type MLTunedParamsEntry struct {
	Model     string
	Format    string
	CreatedAt string
	Metrics   json.RawMessage
}

// HasAnyTunedParamsForModel returns true if at least one tuned-params row exists for the given model (any format).
func HasAnyTunedParamsForModel(ctx context.Context, model string) (bool, error) {
	var n int
	err := Pool.QueryRow(ctx, `
		SELECT 1 FROM ml_tuned_params WHERE model = $1 LIMIT 1
	`, model).Scan(&n)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ListLatestMLTunedParams returns the latest tuned-params entry per (model, format).
// Used so operators can see which model+format combinations have saved params for retraining.
func ListLatestMLTunedParams(ctx context.Context) ([]MLTunedParamsEntry, error) {
	rows, err := Pool.Query(ctx, `
		SELECT DISTINCT ON (model, format) model, format, created_at, metrics
		FROM ml_tuned_params
		ORDER BY model, format, created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MLTunedParamsEntry
	for rows.Next() {
		var e MLTunedParamsEntry
		if err := rows.Scan(&e.Model, &e.Format, &e.CreatedAt, &e.Metrics); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MLTunedParamsByMigrationEntry holds one row from ml_tuned_params joined by data_migration_id.
// Used by ops/migrations/{id}/auto-tune to show per-migration auto-tune details.
type MLTunedParamsByMigrationEntry struct {
	Model     string
	Format    string
	CreatedAt string
	Params    json.RawMessage
	Metrics   json.RawMessage
}

// ListMLTunedParamsByMigration returns all tuned-params rows linked to the given data_migration_id.
// When DB is unavailable or no rows exist, it returns an empty slice and nil error.
func ListMLTunedParamsByMigration(ctx context.Context, migrationID int) ([]MLTunedParamsByMigrationEntry, error) {
	if !Available() || migrationID <= 0 {
		return []MLTunedParamsByMigrationEntry{}, nil
	}
	rows, err := Pool.Query(ctx, `
		SELECT model, format, created_at, params, metrics
		FROM ml_tuned_params
		WHERE data_migration_id = $1
		ORDER BY created_at DESC, model, format
	`, migrationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MLTunedParamsByMigrationEntry
	for rows.Next() {
		var e MLTunedParamsByMigrationEntry
		if err := rows.Scan(&e.Model, &e.Format, &e.CreatedAt, &e.Params, &e.Metrics); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// MigrationInfoForModelStats holds started_at, completed_at, and duration for model-stats enrichment.
type MigrationInfoForModelStats struct {
	TrainedAt    string  // ISO8601
	CompletedAt  string  // ISO8601
	DurationSecs float64 // completed_at - started_at in seconds
}

// ParamsMetricsForModelStats holds params and metrics for model-stats enrichment.
type ParamsMetricsForModelStats struct {
	Params  json.RawMessage
	Metrics json.RawMessage
}

// ListLatestParamsMetricsForModelStats returns the latest params and metrics per (model, format).
// Key format: "model|format" (model lowercased, format uppercased). Used to enrich model-stats with DB data.
func ListLatestParamsMetricsForModelStats(ctx context.Context) (map[string]ParamsMetricsForModelStats, error) {
	if !Available() {
		return make(map[string]ParamsMetricsForModelStats), nil
	}
	rows, err := Pool.Query(ctx, `
		SELECT DISTINCT ON (LOWER(model), UPPER(format)) LOWER(model), UPPER(format), params, metrics
		FROM ml_tuned_params
		ORDER BY LOWER(model), UPPER(format), created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]ParamsMetricsForModelStats)
	for rows.Next() {
		var model, format string
		var params, metrics json.RawMessage
		if err := rows.Scan(&model, &format, &params, &metrics); err != nil {
			return nil, err
		}
		key := model + "|" + format
		out[key] = ParamsMetricsForModelStats{Params: params, Metrics: metrics}
	}
	return out, rows.Err()
}

// GetMigrationInfoForTunedParams returns migration info (trained_at, duration) keyed by "model|format".
// model and format are lower/upper-cased to match ml_tuned_params. Returns empty map when DB unavailable.
func GetMigrationInfoForTunedParams(ctx context.Context) (map[string]MigrationInfoForModelStats, error) {
	if !Available() {
		return make(map[string]MigrationInfoForModelStats), nil
	}
	rows, err := Query(ctx, `
		SELECT DISTINCT ON (LOWER(tp.model), tp.format) LOWER(tp.model), tp.format,
			dm.started_at, dm.completed_at,
			EXTRACT(EPOCH FROM (dm.completed_at - dm.started_at)) AS duration_secs
		FROM ml_tuned_params tp
		JOIN data_migrations dm ON dm.id = tp.data_migration_id
		WHERE tp.data_migration_id IS NOT NULL
		ORDER BY LOWER(tp.model), tp.format, tp.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]MigrationInfoForModelStats)
	for rows.Next() {
		var model, format string
		var startedAt time.Time
		var completedAt *time.Time
		var durationSecs *float64
		if err := rows.Scan(&model, &format, &startedAt, &completedAt, &durationSecs); err != nil {
			return nil, err
		}
		key := model + "|" + format
		info := MigrationInfoForModelStats{
			TrainedAt:    startedAt.UTC().Format(time.RFC3339),
			DurationSecs: 0,
		}
		if completedAt != nil {
			info.CompletedAt = completedAt.UTC().Format(time.RFC3339)
		}
		if durationSecs != nil {
			info.DurationSecs = *durationSecs
		}
		out[key] = info
	}
	return out, rows.Err()
}
