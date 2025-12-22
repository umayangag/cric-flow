package db

import (
    "context"
    "database/sql"
    "errors"
    "time"
)

// GetNextSeasonAfter returns the smallest season id strictly greater than the cutoff year.
// If format is non-empty, it filters seasons to those having matches with that format code.
// Returns sql.NullInt64 with Valid=false when no such season exists.
func GetNextSeasonAfter(ctx context.Context, cutoff time.Time, format string) (sql.NullInt64, error) {
    var out sql.NullInt64
    if Pool == nil {
        return out, errors.New("db pool not initialized")
    }
    year := cutoff.Year()
    // We assume season.id stores the season year (as used across the repo/export queries).
    // Join match_details to season (season_id) and optionally to match_format when format is provided.
    if format == "" {
        err := Pool.QueryRow(ctx, `
            SELECT MIN(s.id) AS next_season
            FROM match_details md
            JOIN season s ON s.id = md.season_id
            WHERE s.id > $1
        `, year).Scan(&out)
        return out, err
    }
    err := Pool.QueryRow(ctx, `
        SELECT MIN(s.id) AS next_season
        FROM match_details md
        JOIN season s ON s.id = md.season_id
        JOIN match_format mf ON mf.id = md.format_id
        WHERE s.id > $1 AND mf.code = $2
    `, year, format).Scan(&out)
    return out, err
}
