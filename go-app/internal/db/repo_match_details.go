package db

import (
	"context"
	"errors"
)

// EnsureMatchByID inserts a match row if it doesn't exist. Uses minimal required fields;
// other jobs can enrich via UpsertMatch. Match table requires format_id, match_date, original_match_type.
// Callers should use EnsureMatchWithFormat when those are available.
func EnsureMatchByID(ctx context.Context, matchID int64) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	// Use placeholder values; match table requires format_id, match_date, original_match_type NOT NULL
	formatID, _ := GetMatchFormatIDByCode(ctx, "T20")
	if formatID <= 0 {
		formatID = 1
	}
	return Exec(ctx, `INSERT INTO match (match_id, format_id, match_date, original_match_type, balls_per_over)
		VALUES ($1, $2, '1970-01-01', 'unknown', 6) ON CONFLICT (match_id) DO NOTHING`, matchID, formatID)
}

// ExistsMatchID checks if a match_id exists in the match table.
func ExistsMatchID(ctx context.Context, matchID int64) (bool, error) {
	if Pool == nil {
		return false, errors.New("db pool not initialized")
	}
	var exists bool
	err := Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match WHERE match_id = $1)`, matchID).Scan(&exists)
	return exists, err
}
