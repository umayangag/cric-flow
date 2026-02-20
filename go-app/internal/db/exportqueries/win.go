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
	// team1 = batting in inning 1, team2 = bowling in inning 1. Same weather + bat/bowl aggregates as batting/bowling/fielding.
	q := `SELECT
		m.match_id,
		m.format_id,
		COALESCE(m.venue_id, 0),
		mi.batting_team_opposition_id AS team1_opposition_id,
		mi.bowling_team_opposition_id AS team2_opposition_id,
		COALESCE(m.toss_winner_opposition_id, 0),
		CASE WHEN m.outcome_winner_opposition_id IS NULL THEN 0 WHEN m.outcome_winner_opposition_id = mi.batting_team_opposition_id THEN 1 ELSE 0 END AS team1_wins,
		COALESCE(mf.code, '') AS format_code,
		COALESCE(w.temp, 0),
		COALESCE(w.wind, 0),
		COALESCE(w.rain, 0),
		COALESCE(w.humidity, 0),
		COALESCE(w.cloud, 0),
		COALESCE(w.pressure, 0),
		CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
		(SELECT COALESCE(SUM(s.v), 0) FROM (
			SELECT DISTINCT ON (bd.player_id) fcs.batting_value AS v
			FROM batting_data bd
			JOIN feature_consistency_snapshots fcs ON fcs.player_id = bd.player_id AND fcs.format_id = m.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= m.match_date
			WHERE bd.match_id = m.match_id AND bd.inning_number = 1
			ORDER BY bd.player_id, fcs.as_of_date DESC
		) s) AS team1_bat_consistency_sum,
		(SELECT COALESCE(SUM(s.v), 0) FROM (
			SELECT DISTINCT ON (bw.player_id) fcs.bowling_value AS v
			FROM bowling_data bw
			JOIN feature_consistency_snapshots fcs ON fcs.player_id = bw.player_id AND fcs.format_id = m.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= m.match_date
			WHERE bw.match_id = m.match_id AND bw.inning_number = 2
			ORDER BY bw.player_id, fcs.as_of_date DESC
		) s) AS team1_bowl_consistency_sum,
		(SELECT COALESCE(SUM(s.v), 0) FROM (
			SELECT DISTINCT ON (bd.player_id) fcs.batting_value AS v
			FROM batting_data bd
			JOIN feature_consistency_snapshots fcs ON fcs.player_id = bd.player_id AND fcs.format_id = m.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= m.match_date
			WHERE bd.match_id = m.match_id AND bd.inning_number = 2
			ORDER BY bd.player_id, fcs.as_of_date DESC
		) s) AS team2_bat_consistency_sum,
		(SELECT COALESCE(SUM(s.v), 0) FROM (
			SELECT DISTINCT ON (bw.player_id) fcs.bowling_value AS v
			FROM bowling_data bw
			JOIN feature_consistency_snapshots fcs ON fcs.player_id = bw.player_id AND fcs.format_id = m.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= m.match_date
			WHERE bw.match_id = m.match_id AND bw.inning_number = 1
			ORDER BY bw.player_id, fcs.as_of_date DESC
		) s) AS team2_bowl_consistency_sum,
		(SELECT COALESCE(SUM(s.v), 0) FROM (
			SELECT DISTINCT ON (bd.player_id) ff.batting_value AS v
			FROM batting_data bd
			JOIN feature_form_snapshots ff ON ff.player_id = bd.player_id AND ff.format_id = m.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= m.match_date
			WHERE bd.match_id = m.match_id AND bd.inning_number = 1
			ORDER BY bd.player_id, ff.as_of_date DESC
		) s) AS team1_bat_form_sum,
		(SELECT COALESCE(SUM(s.v), 0) FROM (
			SELECT DISTINCT ON (bw.player_id) ff.bowling_value AS v
			FROM bowling_data bw
			JOIN feature_form_snapshots ff ON ff.player_id = bw.player_id AND ff.format_id = m.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= m.match_date
			WHERE bw.match_id = m.match_id AND bw.inning_number = 2
			ORDER BY bw.player_id, ff.as_of_date DESC
		) s) AS team1_bowl_form_sum,
		(SELECT COALESCE(SUM(s.v), 0) FROM (
			SELECT DISTINCT ON (bd.player_id) ff.batting_value AS v
			FROM batting_data bd
			JOIN feature_form_snapshots ff ON ff.player_id = bd.player_id AND ff.format_id = m.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= m.match_date
			WHERE bd.match_id = m.match_id AND bd.inning_number = 2
			ORDER BY bd.player_id, ff.as_of_date DESC
		) s) AS team2_bat_form_sum,
		(SELECT COALESCE(SUM(s.v), 0) FROM (
			SELECT DISTINCT ON (bw.player_id) ff.bowling_value AS v
			FROM bowling_data bw
			JOIN feature_form_snapshots ff ON ff.player_id = bw.player_id AND ff.format_id = m.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= m.match_date
			WHERE bw.match_id = m.match_id AND bw.inning_number = 1
			ORDER BY bw.player_id, ff.as_of_date DESC
		) s) AS team2_bowl_form_sum
	FROM match m
	JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
	LEFT JOIN match_format mf ON m.format_id = mf.id
	LEFT JOIN (SELECT match_id, temp, wind, rain, humidity, cloud, pressure, viscosity FROM weather_data WHERE session = 'batting') w ON w.match_id = m.match_id
	WHERE m.match_date < $1`
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
			strconv.Itoa(temp), strconv.Itoa(wind), strconv.Itoa(rain), strconv.Itoa(humidity), strconv.Itoa(cloud), strconv.Itoa(pressure), strconv.Itoa(viscosity),
			strconv.FormatFloat(t1BatCons, 'f', -1, 64), strconv.FormatFloat(t1BowlCons, 'f', -1, 64), strconv.FormatFloat(t2BatCons, 'f', -1, 64), strconv.FormatFloat(t2BowlCons, 'f', -1, 64),
			strconv.FormatFloat(t1BatForm, 'f', -1, 64), strconv.FormatFloat(t1BowlForm, 'f', -1, 64), strconv.FormatFloat(t2BatForm, 'f', -1, 64), strconv.FormatFloat(t2BowlForm, 'f', -1, 64),
		})
	}
	return out, rows.Err()
}
