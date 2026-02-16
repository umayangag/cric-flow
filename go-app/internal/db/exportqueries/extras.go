package exportqueries

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// ExtrasTrainingRows returns match-level rows for extras prediction: format_id, venue_id, season_id, total_extras.
// Used by the backtest training-data API and ML extras model.
func ExtrasTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
	return extrasTrainingRowsImpl(ctx, cutoff, nil)
}

// ExtrasTrainingRowsWithFormat returns extras rows filtered by format code.
func ExtrasTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, err
	}
	return extrasTrainingRowsImpl(ctx, cutoff, formatIDs)
}

func extrasTrainingRowsImpl(ctx context.Context, cutoff time.Time, formatIDs []int64) ([][]string, error) {
	q := `SELECT m.match_id, m.format_id, COALESCE(m.venue_id, 0), COALESCE(m.season_id, 0),
		SUM(mi.extras)::int AS total_extras
		FROM match m
		JOIN match_inning mi ON mi.match_id = m.match_id
		WHERE m.match_date < $1
		GROUP BY m.match_id, m.format_id, m.venue_id, m.season_id`
	args := []any{cutoff}
	if formatIDs != nil {
		q = strings.Replace(
			q,
			"WHERE m.match_date < $1",
			"WHERE m.format_id = ANY($1::bigint[]) AND m.match_date < $2",
			1,
		)
		args = []any{formatIDs, cutoff}
	}
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{"match_id", "format_id", "venue_id", "season_id", "total_extras"}
	out := make([][]string, 0, 256)
	out = append(out, headers)
	for rows.Next() {
		var matchID, formatID, venueID, seasonID int64
		var totalExtras int
		if err := rows.Scan(&matchID, &formatID, &venueID, &seasonID, &totalExtras); err != nil {
			return nil, err
		}
		out = append(out, []string{
			strconv.FormatInt(matchID, 10),
			strconv.FormatInt(formatID, 10),
			strconv.FormatInt(venueID, 10),
			strconv.FormatInt(seasonID, 10),
			strconv.Itoa(totalExtras),
		})
	}
	return out, rows.Err()
}
