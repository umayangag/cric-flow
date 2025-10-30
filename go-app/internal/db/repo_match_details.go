package db

import (
	"context"
	"errors"
)

// EnsureMatchByID inserts a match_details row if it doesn't exist, keyed by match_id.
// It only guarantees presence of the row so other jobs can enrich it later.
func EnsureMatchByID(ctx context.Context, matchID int64) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `INSERT INTO match_details(match_id) VALUES($1)
		ON CONFLICT (match_id) DO NOTHING`, matchID)
	return err
}

// ExistsMatchID checks if a match_id exists in match_details.
func ExistsMatchID(ctx context.Context, matchID int64) (bool, error) {
	if Pool == nil {
		return false, errors.New("db pool not initialized")
	}
	var exists bool
	err := Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match_details WHERE match_id = $1)`, matchID).Scan(&exists)
	return exists, err
}
