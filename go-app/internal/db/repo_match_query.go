package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// GetMatchDate returns match_details.date for a match_id, if present.
func GetMatchDate(ctx context.Context, matchID int64) (*time.Time, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	var d sql.NullTime
	err := Pool.QueryRow(ctx, `SELECT date FROM match_details WHERE match_id = $1`, matchID).Scan(&d)
	if err != nil {
		return nil, err
	}
	if d.Valid {
		return &d.Time, nil
	}
	return nil, nil
}
