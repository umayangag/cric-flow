package exportqueries

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// ExtrasTrainingRows returns match-level rows for extras prediction: format_id, venue_id, season_id, total_extras,
// plus weather (temp, wind, rain, humidity, cloud, pressure, viscosity) and match-level bat/bowl consistency and form sums.
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
	// Match-level row with weather and aggregated batting/bowling features (same feature families as batting/bowling/fielding).
	q := `SELECT
		m.match_id,
		m.format_id,
		COALESCE(m.venue_id, 0),
		COALESCE(m.season_id, 0),
		SUM(mi.extras)::int AS total_extras,
		COALESCE(mf.code, '') AS format_code,
		COALESCE(w.temp, 0),
		COALESCE(w.wind, 0),
		COALESCE(w.rain, 0),
		COALESCE(w.humidity, 0),
		COALESCE(w.cloud, 0),
		COALESCE(w.pressure, 0),
		CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
		(SELECT COALESCE(SUM(snap.v), 0)
		 FROM batting_data bd
		 LEFT JOIN LATERAL (
			SELECT fcs.batting_value AS v
			FROM feature_consistency_snapshots fcs
			WHERE fcs.player_id = bd.player_id AND fcs.format_id = m.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= m.match_date
			ORDER BY fcs.as_of_date DESC LIMIT 1
		 ) snap ON TRUE
		 WHERE bd.match_id = m.match_id
		) AS bat_consistency_sum,
		(SELECT COALESCE(SUM(snap.v), 0)
		 FROM bowling_data bw
		 LEFT JOIN LATERAL (
			SELECT fcs.bowling_value AS v
			FROM feature_consistency_snapshots fcs
			WHERE fcs.player_id = bw.player_id AND fcs.format_id = m.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= m.match_date
			ORDER BY fcs.as_of_date DESC LIMIT 1
		 ) snap ON TRUE
		 WHERE bw.match_id = m.match_id
		) AS bowl_consistency_sum,
		(SELECT COALESCE(SUM(snap.v), 0)
		 FROM batting_data bd
		 LEFT JOIN LATERAL (
			SELECT ff.batting_value AS v
			FROM feature_form_snapshots ff
			WHERE ff.player_id = bd.player_id AND ff.format_id = m.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= m.match_date
			ORDER BY ff.as_of_date DESC LIMIT 1
		 ) snap ON TRUE
		 WHERE bd.match_id = m.match_id
		) AS bat_form_sum,
		(SELECT COALESCE(SUM(snap.v), 0)
		 FROM bowling_data bw
		 LEFT JOIN LATERAL (
			SELECT ff.bowling_value AS v
			FROM feature_form_snapshots ff
			WHERE ff.player_id = bw.player_id AND ff.format_id = m.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= m.match_date
			ORDER BY ff.as_of_date DESC LIMIT 1
		 ) snap ON TRUE
		 WHERE bw.match_id = m.match_id
		) AS bowl_form_sum
	FROM match m
	JOIN match_inning mi ON mi.match_id = m.match_id
	LEFT JOIN match_format mf ON m.format_id = mf.id
	LEFT JOIN (SELECT match_id, temp, wind, rain, humidity, cloud, pressure, viscosity FROM weather_data WHERE session = 'batting') w ON w.match_id = m.match_id
	WHERE m.match_date < $1
	GROUP BY m.match_id, m.format_id, m.venue_id, m.season_id, mf.code, w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure, w.viscosity`
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
	headers := []string{
		"match_id", "format_id", "venue_id", "season_id", "total_extras", "format_code",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"bat_consistency_sum", "bowl_consistency_sum", "bat_form_sum", "bowl_form_sum",
	}
	out := make([][]string, 0, 256)
	out = append(out, headers)
	for rows.Next() {
		var matchID, formatID, venueID, seasonID int64
		var totalExtras int
		var formatCode string
		var temp, wind, rain, humidity, cloud, pressure, viscosity int
		var batConsSum, bowlConsSum, batFormSum, bowlFormSum float64
		if err := rows.Scan(&matchID, &formatID, &venueID, &seasonID, &totalExtras, &formatCode,
			&temp, &wind, &rain, &humidity, &cloud, &pressure, &viscosity,
			&batConsSum, &bowlConsSum, &batFormSum, &bowlFormSum); err != nil {
			return nil, err
		}
		out = append(out, []string{
			strconv.FormatInt(matchID, 10),
			strconv.FormatInt(formatID, 10),
			strconv.FormatInt(venueID, 10),
			strconv.FormatInt(seasonID, 10),
			strconv.Itoa(totalExtras),
			formatCode,
			strconv.Itoa(temp), strconv.Itoa(wind), strconv.Itoa(rain), strconv.Itoa(humidity), strconv.Itoa(cloud), strconv.Itoa(pressure), strconv.Itoa(viscosity),
			strconv.FormatFloat(batConsSum, 'f', -1, 64), strconv.FormatFloat(bowlConsSum, 'f', -1, 64), strconv.FormatFloat(batFormSum, 'f', -1, 64), strconv.FormatFloat(bowlFormSum, 'f', -1, 64),
		})
	}
	return out, rows.Err()
}
