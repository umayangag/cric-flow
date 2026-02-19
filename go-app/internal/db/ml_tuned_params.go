package db

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// InsertMLTunedParams inserts a row into ml_tuned_params. params is stored as JSONB.
func InsertMLTunedParams(ctx context.Context, model, format string, params json.RawMessage) error {
	_, err := Pool.Exec(ctx, `
		INSERT INTO ml_tuned_params (model, format, params)
		VALUES ($1, $2, $3)
	`, model, format, params)
	return err
}

// MLTunedParamsRow is the row returned by GetLatestMLTunedParams.
type MLTunedParamsRow struct {
	Params    json.RawMessage
	CreatedAt string
}

// GetLatestMLTunedParams returns the most recent params for the given model and format.
// format may be empty to mean "any"; the query returns the latest row for (model, format).
// Returns (nil, nil) when no row exists.
func GetLatestMLTunedParams(ctx context.Context, model, format string) (*MLTunedParamsRow, error) {
	var params json.RawMessage
	var createdAt string
	err := Pool.QueryRow(ctx, `
		SELECT params, created_at
		FROM ml_tuned_params
		WHERE model = $1 AND (format = $2 OR ($2 = '' AND format = ''))
		ORDER BY created_at DESC
		LIMIT 1
	`, model, format).Scan(&params, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &MLTunedParamsRow{Params: params, CreatedAt: createdAt}, nil
}
