package db

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// InsertMLTunedParams inserts a row into ml_tuned_params. params and metrics are stored as JSONB.
// metrics may be nil; a nil value is stored as NULL.
func InsertMLTunedParams(ctx context.Context, model, format string, params json.RawMessage, metrics json.RawMessage) error {
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
