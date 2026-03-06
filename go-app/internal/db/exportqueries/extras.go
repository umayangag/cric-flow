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
	// Match-level row with weather and aggregated batting/bowling features. Uses CTEs to avoid
	// correlated subqueries: pre-aggregate per match via CTEs, then join once.
	q := `WITH matches_filtered AS (
		SELECT m.match_id, m.format_id, COALESCE(m.venue_id, 0) AS venue_id, COALESCE(m.season_id, 0) AS season_id,
			m.match_date, COALESCE(mf.code, '') AS format_code,
			COALESCE(w.temp, 0) AS temp, COALESCE(w.wind, 0) AS wind, COALESCE(w.rain, 0) AS rain,
			COALESCE(w.humidity, 0) AS humidity, COALESCE(w.cloud, 0) AS cloud, COALESCE(w.pressure, 0) AS pressure,
			CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
			SUM(mi.extras)::int AS total_extras
		FROM match m
		JOIN match_inning mi ON mi.match_id = m.match_id
		LEFT JOIN match_format mf ON m.format_id = mf.id
LEFT JOIN (SELECT DISTINCT ON (match_id) match_id, temp, wind, rain, humidity, cloud, pressure, viscosity FROM weather_data WHERE session = 'batting' ORDER BY match_id, id DESC) w ON w.match_id = m.match_id
		WHERE m.match_date < $1
		GROUP BY m.match_id, m.format_id, m.venue_id, m.season_id, mf.code, w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure, w.viscosity
	),
	bat_players AS (
		SELECT bd.match_id, bd.player_id, m.format_id, m.match_date
		FROM batting_data bd
		JOIN matches_filtered m ON m.match_id = bd.match_id
	),
	bowl_players AS (
		SELECT bw.match_id, bw.player_id, m.format_id, m.match_date
		FROM bowling_data bw
		JOIN matches_filtered m ON m.match_id = bw.match_id
	),
	bat_consistency AS (
		SELECT DISTINCT ON (r.player_id, r.format_id, bp.match_id)
			bp.match_id, r.batting_std_w10 AS v
		FROM bat_players bp
		JOIN feature_raw_stats_snapshots r ON r.player_id = bp.player_id AND r.format_id = bp.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date <= bp.match_date
		ORDER BY r.player_id, r.format_id, bp.match_id, r.as_of_date DESC
	),
	bowl_consistency AS (
		SELECT DISTINCT ON (r.player_id, r.format_id, bp.match_id)
			bp.match_id, r.bowling_std_w10 AS v
		FROM bowl_players bp
		JOIN feature_raw_stats_snapshots r ON r.player_id = bp.player_id AND r.format_id = bp.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date <= bp.match_date
		ORDER BY r.player_id, r.format_id, bp.match_id, r.as_of_date DESC
	),
	bat_form AS (
		SELECT DISTINCT ON (r.player_id, r.format_id, bp.match_id)
			bp.match_id, r.batting_mean_w5 AS v
		FROM bat_players bp
		JOIN feature_raw_stats_snapshots r ON r.player_id = bp.player_id AND r.format_id = bp.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date <= bp.match_date
		ORDER BY r.player_id, r.format_id, bp.match_id, r.as_of_date DESC
	),
	bowl_form AS (
		SELECT DISTINCT ON (r.player_id, r.format_id, bp.match_id)
			bp.match_id, r.bowling_mean_w5 AS v
		FROM bowl_players bp
		JOIN feature_raw_stats_snapshots r ON r.player_id = bp.player_id AND r.format_id = bp.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date <= bp.match_date
		ORDER BY r.player_id, r.format_id, bp.match_id, r.as_of_date DESC
	),
	bat_cons_agg AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM bat_consistency GROUP BY match_id),
	bowl_cons_agg AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM bowl_consistency GROUP BY match_id),
	bat_form_agg AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM bat_form GROUP BY match_id),
	bowl_form_agg AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM bowl_form GROUP BY match_id)
	SELECT m.match_id, m.format_id, m.venue_id, m.season_id, m.total_extras, m.format_code,
		m.match_date,
		m.temp, m.wind, m.rain, m.humidity, m.cloud, m.pressure, m.viscosity,
		COALESCE(bc.s, 0) AS bat_consistency_sum,
		COALESCE(bwc.s, 0) AS bowl_consistency_sum,
		COALESCE(bf.s, 0) AS bat_form_sum,
		COALESCE(bwf.s, 0) AS bowl_form_sum
	FROM matches_filtered m
	LEFT JOIN bat_cons_agg bc ON bc.match_id = m.match_id
	LEFT JOIN bowl_cons_agg bwc ON bwc.match_id = m.match_id
	LEFT JOIN bat_form_agg bf ON bf.match_id = m.match_id
	LEFT JOIN bowl_form_agg bwf ON bwf.match_id = m.match_id
	ORDER BY m.match_date ASC, m.match_id`
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
		"match_date", "match_date_unix",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"bat_consistency_sum", "bowl_consistency_sum", "bat_form_sum", "bowl_form_sum",
	}
	out := make([][]string, 0, 256)
	out = append(out, headers)
	for rows.Next() {
		var matchID, formatID, venueID, seasonID int64
		var totalExtras int
		var formatCode string
		var matchDate time.Time
		var temp, wind, rain, humidity, cloud, pressure, viscosity int
		var batConsSum, bowlConsSum, batFormSum, bowlFormSum float64
		if err := rows.Scan(&matchID, &formatID, &venueID, &seasonID, &totalExtras, &formatCode,
			&matchDate,
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
			matchDate.Format("2006-01-02"),
			strconv.FormatInt(matchDate.Unix(), 10),
			strconv.Itoa(
				temp,
			), strconv.Itoa(wind), strconv.Itoa(rain), strconv.Itoa(humidity), strconv.Itoa(cloud), strconv.Itoa(pressure), strconv.Itoa(viscosity),
			strconv.FormatFloat(
				batConsSum,
				'f',
				-1,
				64,
			), strconv.FormatFloat(bowlConsSum, 'f', -1, 64), strconv.FormatFloat(batFormSum, 'f', -1, 64), strconv.FormatFloat(bowlFormSum, 'f', -1, 64),
		})
	}
	return out, rows.Err()
}
