package db

import (
	"context"
	"errors"
)

// EnqueueMissingWeatherJobs inserts weather_job rows for matches that don't have a job yet.
// It uses venue.venue_name as the source for normalized_venue (lower-cased collapsed);
// city/country/time fields are left null for now. Limit controls max rows to enqueue; if <=0, no limit.
func EnqueueMissingWeatherJobs(ctx context.Context, limit int) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	limClause := ""
	if limit > 0 {
		limClause = " LIMIT " + itoa(limit)
	}
	// Insert-Select idempotent via unique(match_id)
	cmd := `INSERT INTO weather_job(match_id, normalized_venue, sessions, status)
		SELECT md.match_id,
		       regexp_replace(lower(trim(v.venue_name)), '\\s+', ' ', 'g') AS normalized_venue,
		       '[]'::jsonb,
		       'queued'
		FROM match_details md
		JOIN venue v ON v.id = md.venue_id
		LEFT JOIN weather_job wj ON wj.match_id = md.match_id
		WHERE wj.match_id IS NULL` + limClause
	ct, err := Pool.Exec(ctx, cmd)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func itoa(n int) string {
	// simple int to string to avoid fmt import
	if n == 0 { return "0" }
	sign := ""
	if n < 0 { sign = "-"; n = -n }
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return sign + string(buf[i:])
}
