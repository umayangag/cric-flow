package exportqueries

import (
	"context"
	"strconv"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// InningsTrainingRows returns innings-level rows for the innings model (runs, wickets per innings).
// One row per (match_id, inning_number). Used for hybrid reconciliation: innings model predicts
// innings_runs and innings_wickets; player predictions are rescaled to match.
// Features: format_code (categorical), venue_id, season_id, inning_number, opposition_id (batting team's opposition),
// bat_consistency_sum (batting team), bowl_consistency_sum (bowling team), bat_form_sum, bowl_form_sum.
func InningsTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
	return inningsTrainingRowsImpl(ctx, cutoff, nil)
}

// InningsTrainingRowsWithFormat returns innings rows filtered by format code.
func InningsTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, err
	}
	return inningsTrainingRowsImpl(ctx, cutoff, formatIDs)
}

func inningsTrainingRowsImpl(ctx context.Context, cutoff time.Time, formatIDs []int64) ([][]string, error) {
	// Per innings: match_id, inning_number, innings_runs, innings_wickets, venue_id, season_id,
	// opposition_id (batting team's opposition = bowling team), format_code, bat/bowl consistency and form sums.
	whereClause := "m.match_date < $1"
	args := []any{cutoff}
	if formatIDs != nil {
		whereClause = "m.format_id = ANY($1::bigint[]) AND m.match_date < $2"
		args = []any{formatIDs, cutoff}
	}
	q := `WITH innings_filtered AS (
		SELECT mi.match_id, mi.inning_number,
			COALESCE(bd.runs, 0)::int AS innings_runs,
			COALESCE(bw.wickets, 0)::int AS innings_wickets,
			m.format_id, COALESCE(m.venue_id, 0) AS venue_id,			mi.bowling_team_opposition_id AS opposition_id,
			COALESCE(mf.code, '') AS format_code,
			m.match_date
		FROM match_inning mi
		JOIN match m ON m.match_id = mi.match_id
		LEFT JOIN match_format mf ON m.format_id = mf.id
		LEFT JOIN (SELECT match_id, inning_number, SUM(runs)::int AS runs FROM batting_data GROUP BY match_id, inning_number) bd ON bd.match_id = mi.match_id AND bd.inning_number = mi.inning_number
		LEFT JOIN (SELECT match_id, inning_number, SUM(wickets)::int AS wickets FROM bowling_data GROUP BY match_id, inning_number) bw ON bw.match_id = mi.match_id AND bw.inning_number = mi.inning_number
		WHERE ` + whereClause + `
	),
	bat_players AS (
		SELECT bd.match_id, bd.inning_number, bd.player_id, i.format_id, i.match_date
		FROM batting_data bd
		JOIN innings_filtered i ON i.match_id = bd.match_id AND i.inning_number = bd.inning_number
	),
	bowl_players AS (
		SELECT bw.match_id, bw.inning_number, bw.player_id, i.format_id, i.match_date
		FROM bowling_data bw
		JOIN innings_filtered i ON i.match_id = bw.match_id AND i.inning_number = bw.inning_number
	),
	bat_features_raw AS (
		(SELECT match_id, inning_number, 'consistency'::text AS kind, v FROM (
			SELECT DISTINCT ON (r.player_id, r.format_id, bp.match_id, bp.inning_number)
				bp.match_id, bp.inning_number, r.batting_std_w10 AS v
			FROM bat_players bp
			JOIN feature_raw_stats_snapshots r ON r.player_id = bp.player_id AND r.format_id = bp.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date <= bp.match_date
			ORDER BY r.player_id, r.format_id, bp.match_id, bp.inning_number, r.as_of_date DESC
		) x)
		UNION ALL
		(SELECT match_id, inning_number, 'form'::text AS kind, v FROM (
			SELECT DISTINCT ON (r.player_id, r.format_id, bp.match_id, bp.inning_number)
				bp.match_id, bp.inning_number, r.batting_mean_w5 AS v
			FROM bat_players bp
			JOIN feature_raw_stats_snapshots r ON r.player_id = bp.player_id AND r.format_id = bp.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date <= bp.match_date
			ORDER BY r.player_id, r.format_id, bp.match_id, bp.inning_number, r.as_of_date DESC
		) y)
	),
	bowl_features_raw AS (
		(SELECT match_id, inning_number, 'consistency'::text AS kind, v FROM (
			SELECT DISTINCT ON (r.player_id, r.format_id, bp.match_id, bp.inning_number)
				bp.match_id, bp.inning_number, r.bowling_std_w10 AS v
			FROM bowl_players bp
			JOIN feature_raw_stats_snapshots r ON r.player_id = bp.player_id AND r.format_id = bp.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date <= bp.match_date
			ORDER BY r.player_id, r.format_id, bp.match_id, bp.inning_number, r.as_of_date DESC
		) x)
		UNION ALL
		(SELECT match_id, inning_number, 'form'::text AS kind, v FROM (
			SELECT DISTINCT ON (r.player_id, r.format_id, bp.match_id, bp.inning_number)
				bp.match_id, bp.inning_number, r.bowling_mean_w5 AS v
			FROM bowl_players bp
			JOIN feature_raw_stats_snapshots r ON r.player_id = bp.player_id AND r.format_id = bp.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date <= bp.match_date
			ORDER BY r.player_id, r.format_id, bp.match_id, bp.inning_number, r.as_of_date DESC
		) y)
	),
	bat_cons_agg AS (SELECT match_id, inning_number, COALESCE(SUM(v), 0) AS s FROM bat_features_raw WHERE kind = 'consistency' GROUP BY match_id, inning_number),
	bowl_cons_agg AS (SELECT match_id, inning_number, COALESCE(SUM(v), 0) AS s FROM bowl_features_raw WHERE kind = 'consistency' GROUP BY match_id, inning_number),
	bat_form_agg AS (SELECT match_id, inning_number, COALESCE(SUM(v), 0) AS s FROM bat_features_raw WHERE kind = 'form' GROUP BY match_id, inning_number),
	bowl_form_agg AS (SELECT match_id, inning_number, COALESCE(SUM(v), 0) AS s FROM bowl_features_raw WHERE kind = 'form' GROUP BY match_id, inning_number)
	SELECT i.match_id, i.inning_number, i.innings_runs, i.innings_wickets, i.venue_id, i.opposition_id, i.format_code,
		i.match_date,
		COALESCE(bc.s, 0) AS bat_consistency_sum,
		COALESCE(bwc.s, 0) AS bowl_consistency_sum,
		COALESCE(bf.s, 0) AS bat_form_sum,
		COALESCE(bwf.s, 0) AS bowl_form_sum
	FROM innings_filtered i
	LEFT JOIN bat_cons_agg bc ON bc.match_id = i.match_id AND bc.inning_number = i.inning_number
	LEFT JOIN bowl_cons_agg bwc ON bwc.match_id = i.match_id AND bwc.inning_number = i.inning_number
	LEFT JOIN bat_form_agg bf ON bf.match_id = i.match_id AND bf.inning_number = i.inning_number
	LEFT JOIN bowl_form_agg bwf ON bwf.match_id = i.match_id AND bwf.inning_number = i.inning_number
	ORDER BY i.match_date ASC, i.match_id, i.inning_number`
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := InningsTrainingHeaders()
	out := make([][]string, 0, 512)
	out = append(out, headers)
	for rows.Next() {
		var matchID, inningNum, inningsRuns, inningsWickets, venueID, oppositionID int64
		var formatCode string
		var matchDate time.Time
		var batConsSum, bowlConsSum, batFormSum, bowlFormSum float64
		if err := rows.Scan(&matchID, &inningNum, &inningsRuns, &inningsWickets, &venueID, &oppositionID, &formatCode,
			&matchDate,
			&batConsSum, &bowlConsSum, &batFormSum, &bowlFormSum); err != nil {
			return nil, err
		}
		out = append(out, []string{
			strconv.FormatInt(matchID, 10),
			strconv.FormatInt(inningNum, 10),
			strconv.FormatInt(inningsRuns, 10),
			strconv.FormatInt(inningsWickets, 10),
			strconv.FormatInt(venueID, 10),
			strconv.FormatInt(oppositionID, 10),
			formatCode,
			matchDate.Format("2006-01-02"),
			strconv.FormatFloat(batConsSum, 'f', -1, 64),
			strconv.FormatFloat(bowlConsSum, 'f', -1, 64),
			strconv.FormatFloat(batFormSum, 'f', -1, 64),
			strconv.FormatFloat(bowlFormSum, 'f', -1, 64),
		})
	}
	return out, rows.Err()
}
