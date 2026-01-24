package db

import (
    "context"
    "errors"
    "github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

// GetMatchFormatIDByCode returns the id from match_format for the provided code.
// It accepts canonical codes (TEST, ODI, T20, T20I) and aliases (MDM→TEST, ODM→ODI, IT20→T20I).
func GetMatchFormatIDByCode(ctx context.Context, code string) (int64, error) {
    if Pool == nil {
        return 0, errors.New("db pool not initialized")
    }
    // Map aliases to canonical codes before querying the dimension table.
    code = formats.CanonicalizeCode(code)
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
