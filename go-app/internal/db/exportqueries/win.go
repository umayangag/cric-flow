package exportqueries

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// WinTrainingRows returns match-level rows for win prediction: format_id, venue_id, team1_opposition_id, team2_opposition_id, toss_winner_opposition_id, team1_wins (0/1),
// plus weather and team-level bat/bowl consistency and form sums (team1 = batting inn 1, team2 = bowling inn 1).
// team1_wins = 1 if outcome_winner_opposition_id == team1 else 0.
func WinTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
	return winTrainingRowsImpl(ctx, cutoff, nil)
}

// WinTrainingRowsWithFormat returns win rows filtered by format code.
func WinTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, err
	}
	return winTrainingRowsImpl(ctx, cutoff, formatIDs)
}

func winTrainingRowsImpl(ctx context.Context, cutoff time.Time, formatIDs []int64) ([][]string, error) {
	// team1 = batting in inning 1, team2 = bowling in inning 1. Uses CTEs to avoid correlated
	// subqueries: pre-aggregate per match/team via CTEs, then join once.
	q := `WITH matches_filtered AS (
		SELECT m.match_id, m.format_id, COALESCE(m.venue_id, 0) AS venue_id,
			mi.batting_team_opposition_id AS team1_opposition_id,
			mi.bowling_team_opposition_id AS team2_opposition_id,
			COALESCE(m.toss_winner_opposition_id, 0) AS toss_winner_opposition_id,
			CASE WHEN m.outcome_winner_opposition_id IS NULL THEN 0 WHEN m.outcome_winner_opposition_id = mi.batting_team_opposition_id THEN 1 ELSE 0 END AS team1_wins,
			COALESCE(mf.code, '') AS format_code,
			COALESCE(w.temp, 0) AS temp, COALESCE(w.wind, 0) AS wind, COALESCE(w.rain, 0) AS rain,
			COALESCE(w.humidity, 0) AS humidity, COALESCE(w.cloud, 0) AS cloud, COALESCE(w.pressure, 0) AS pressure,
			CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
			m.match_date
		FROM match m
		JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
		LEFT JOIN match_format mf ON m.format_id = mf.id
LEFT JOIN (SELECT DISTINCT ON (match_id) match_id, temp, wind, rain, humidity, cloud, pressure, viscosity FROM weather_data WHERE session = 'batting' ORDER BY match_id, id DESC) w ON w.match_id = m.match_id
		WHERE m.match_date < $1
	),
	t1_bat AS (SELECT bd.match_id, bd.player_id, m.format_id, m.match_date FROM batting_data bd JOIN matches_filtered m ON m.match_id = bd.match_id WHERE bd.inning_number = 1),
	t1_bowl AS (SELECT bw.match_id, bw.player_id, m.format_id, m.match_date FROM bowling_data bw JOIN matches_filtered m ON m.match_id = bw.match_id WHERE bw.inning_number = 2),
	t2_bat AS (SELECT bd.match_id, bd.player_id, m.format_id, m.match_date FROM batting_data bd JOIN matches_filtered m ON m.match_id = bd.match_id WHERE bd.inning_number = 2),
	t2_bowl AS (SELECT bw.match_id, bw.player_id, m.format_id, m.match_date FROM bowling_data bw JOIN matches_filtered m ON m.match_id = bw.match_id WHERE bw.inning_number = 1),
	t1_bat_cons AS (
		SELECT DISTINCT ON (fcs.player_id, fcs.format_id, p.match_id) p.match_id, fcs.batting_value AS v
		FROM t1_bat p
		JOIN feature_consistency_snapshots fcs ON fcs.player_id = p.player_id AND fcs.format_id = p.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= p.match_date
		ORDER BY fcs.player_id, fcs.format_id, p.match_id, fcs.as_of_date DESC
	),
	t1_bowl_cons AS (
		SELECT DISTINCT ON (fcs.player_id, fcs.format_id, p.match_id) p.match_id, fcs.bowling_value AS v
		FROM t1_bowl p
		JOIN feature_consistency_snapshots fcs ON fcs.player_id = p.player_id AND fcs.format_id = p.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= p.match_date
		ORDER BY fcs.player_id, fcs.format_id, p.match_id, fcs.as_of_date DESC
	),
	t2_bat_cons AS (
		SELECT DISTINCT ON (fcs.player_id, fcs.format_id, p.match_id) p.match_id, fcs.batting_value AS v
		FROM t2_bat p
		JOIN feature_consistency_snapshots fcs ON fcs.player_id = p.player_id AND fcs.format_id = p.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= p.match_date
		ORDER BY fcs.player_id, fcs.format_id, p.match_id, fcs.as_of_date DESC
	),
	t2_bowl_cons AS (
		SELECT DISTINCT ON (fcs.player_id, fcs.format_id, p.match_id) p.match_id, fcs.bowling_value AS v
		FROM t2_bowl p
		JOIN feature_consistency_snapshots fcs ON fcs.player_id = p.player_id AND fcs.format_id = p.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= p.match_date
		ORDER BY fcs.player_id, fcs.format_id, p.match_id, fcs.as_of_date DESC
	),
	t1_bat_form AS (
		SELECT DISTINCT ON (ff.player_id, ff.format_id, p.match_id) p.match_id, ff.batting_value AS v
		FROM t1_bat p
		JOIN feature_form_snapshots ff ON ff.player_id = p.player_id AND ff.format_id = p.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= p.match_date
		ORDER BY ff.player_id, ff.format_id, p.match_id, ff.as_of_date DESC
	),
	t1_bowl_form AS (
		SELECT DISTINCT ON (ff.player_id, ff.format_id, p.match_id) p.match_id, ff.bowling_value AS v
		FROM t1_bowl p
		JOIN feature_form_snapshots ff ON ff.player_id = p.player_id AND ff.format_id = p.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= p.match_date
		ORDER BY ff.player_id, ff.format_id, p.match_id, ff.as_of_date DESC
	),
	t2_bat_form AS (
		SELECT DISTINCT ON (ff.player_id, ff.format_id, p.match_id) p.match_id, ff.batting_value AS v
		FROM t2_bat p
		JOIN feature_form_snapshots ff ON ff.player_id = p.player_id AND ff.format_id = p.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= p.match_date
		ORDER BY ff.player_id, ff.format_id, p.match_id, ff.as_of_date DESC
	),
	t2_bowl_form AS (
		SELECT DISTINCT ON (ff.player_id, ff.format_id, p.match_id) p.match_id, ff.bowling_value AS v
		FROM t2_bowl p
		JOIN feature_form_snapshots ff ON ff.player_id = p.player_id AND ff.format_id = p.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= p.match_date
		ORDER BY ff.player_id, ff.format_id, p.match_id, ff.as_of_date DESC
	),
	agg_t1_bat_cons AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM t1_bat_cons GROUP BY match_id),
	agg_t1_bowl_cons AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM t1_bowl_cons GROUP BY match_id),
	agg_t2_bat_cons AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM t2_bat_cons GROUP BY match_id),
	agg_t2_bowl_cons AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM t2_bowl_cons GROUP BY match_id),
	agg_t1_bat_form AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM t1_bat_form GROUP BY match_id),
	agg_t1_bowl_form AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM t1_bowl_form GROUP BY match_id),
	agg_t2_bat_form AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM t2_bat_form GROUP BY match_id),
	agg_t2_bowl_form AS (SELECT match_id, COALESCE(SUM(v), 0) AS s FROM t2_bowl_form GROUP BY match_id)
	SELECT m.match_id, m.format_id, m.venue_id, m.team1_opposition_id, m.team2_opposition_id, m.toss_winner_opposition_id, m.team1_wins, m.format_code,
		m.temp, m.wind, m.rain, m.humidity, m.cloud, m.pressure, m.viscosity,
		COALESCE(a1.s, 0) AS team1_bat_consistency_sum,
		COALESCE(a2.s, 0) AS team1_bowl_consistency_sum,
		COALESCE(a3.s, 0) AS team2_bat_consistency_sum,
		COALESCE(a4.s, 0) AS team2_bowl_consistency_sum,
		COALESCE(a5.s, 0) AS team1_bat_form_sum,
		COALESCE(a6.s, 0) AS team1_bowl_form_sum,
		COALESCE(a7.s, 0) AS team2_bat_form_sum,
		COALESCE(a8.s, 0) AS team2_bowl_form_sum
	FROM matches_filtered m
	LEFT JOIN agg_t1_bat_cons a1 ON a1.match_id = m.match_id
	LEFT JOIN agg_t1_bowl_cons a2 ON a2.match_id = m.match_id
	LEFT JOIN agg_t2_bat_cons a3 ON a3.match_id = m.match_id
	LEFT JOIN agg_t2_bowl_cons a4 ON a4.match_id = m.match_id
	LEFT JOIN agg_t1_bat_form a5 ON a5.match_id = m.match_id
	LEFT JOIN agg_t1_bowl_form a6 ON a6.match_id = m.match_id
	LEFT JOIN agg_t2_bat_form a7 ON a7.match_id = m.match_id
	LEFT JOIN agg_t2_bowl_form a8 ON a8.match_id = m.match_id`
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
		"match_id", "format_id", "venue_id", "team1_opposition_id", "team2_opposition_id", "toss_winner_opposition_id", "team1_wins", "format_code",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"team1_bat_consistency_sum", "team1_bowl_consistency_sum", "team2_bat_consistency_sum", "team2_bowl_consistency_sum",
		"team1_bat_form_sum", "team1_bowl_form_sum", "team2_bat_form_sum", "team2_bowl_form_sum",
	}
	out := make([][]string, 0, 256)
	out = append(out, headers)
	for rows.Next() {
		var matchID, formatID, venueID, team1, team2, tossWinner int64
		var team1Wins int
		var formatCode string
		var temp, wind, rain, humidity, cloud, pressure, viscosity int
		var t1BatCons, t1BowlCons, t2BatCons, t2BowlCons float64
		var t1BatForm, t1BowlForm, t2BatForm, t2BowlForm float64
		if err := rows.Scan(&matchID, &formatID, &venueID, &team1, &team2, &tossWinner, &team1Wins, &formatCode,
			&temp, &wind, &rain, &humidity, &cloud, &pressure, &viscosity,
			&t1BatCons, &t1BowlCons, &t2BatCons, &t2BowlCons,
			&t1BatForm, &t1BowlForm, &t2BatForm, &t2BowlForm); err != nil {
			return nil, err
		}
		out = append(out, []string{
			strconv.FormatInt(matchID, 10),
			strconv.FormatInt(formatID, 10),
			strconv.FormatInt(venueID, 10),
			strconv.FormatInt(team1, 10),
			strconv.FormatInt(team2, 10),
			strconv.FormatInt(tossWinner, 10),
			strconv.Itoa(team1Wins),
			formatCode,
			strconv.Itoa(
				temp,
			), strconv.Itoa(wind), strconv.Itoa(rain), strconv.Itoa(humidity), strconv.Itoa(cloud), strconv.Itoa(pressure), strconv.Itoa(viscosity),
			strconv.FormatFloat(
				t1BatCons,
				'f',
				-1,
				64,
			), strconv.FormatFloat(t1BowlCons, 'f', -1, 64), strconv.FormatFloat(t2BatCons, 'f', -1, 64), strconv.FormatFloat(t2BowlCons, 'f', -1, 64),
			strconv.FormatFloat(
				t1BatForm,
				'f',
				-1,
				64,
			), strconv.FormatFloat(t1BowlForm, 'f', -1, 64), strconv.FormatFloat(t2BatForm, 'f', -1, 64), strconv.FormatFloat(t2BowlForm, 'f', -1, 64),
		})
	}
	return out, rows.Err()
}
