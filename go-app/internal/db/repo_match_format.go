package db

import (
	"context"
	"errors"
)

// GetMatchFormatIDByCode returns the id from match_format given a code like "TEST", "ODI", "T20", "T20I".
func GetMatchFormatIDByCode(ctx context.Context, code string) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	var id int64
	err := Pool.QueryRow(ctx, `SELECT id FROM match_format WHERE code = $1`, code).Scan(&id)
	return id, err
}

// EnsureMatchWithFormat inserts a match_details row with required format_id if it doesn't exist.
func EnsureMatchWithFormat(ctx context.Context, matchID int64, formatID int64) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `INSERT INTO match_details(match_id, format_id) VALUES($1, $2)
		ON CONFLICT (match_id) DO NOTHING`, matchID, formatID)
	return err
}
