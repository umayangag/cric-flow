package db

import (
	"context"
	"errors"

	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// GetMatchFormatIDByCode returns the id from match_format for the provided code.
// It accepts canonical codes (TEST, ODI, T20, T20I) and aliases (MDM→TEST, ODM→ODI, IT20→T20I).
func GetMatchFormatIDByCode(ctx context.Context, code string) (int64, error) {
	// PoolAPI, not Pool: this is a lookup the importer makes, and EntityCache used to
	// turn a nil pool into (0, nil) rather than report it (IMPORT-13). It reports it now,
	// so the seam a test injects has to be the one RunInTx already uses.
	if PoolAPI == nil {
		return 0, errors.New("db pool not initialized")
	}
	// Map aliases to canonical codes before querying the dimension table.
	code = formats.CanonicalizeCode(code)
	var id int64
	err := PoolAPI.QueryRow(ctx, `SELECT id FROM match_format WHERE code = $1`, code).Scan(&id)
	return id, err
}
