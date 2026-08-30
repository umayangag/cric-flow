package exportqueries

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/scanx"
	"github.com/umayangag/cric-flow/go-app/internal/features"
)

// BattingUnifiedRows returns CSV-shaped rows for the unified batting export.
// Base batting row + fielding; form/consistency as-of columns were removed in favor of raw stats (per-format export).
func BattingUnifiedRows(ctx context.Context) ([][]string, error) {
	q := `
	SELECT 
	  bd.runs, bd.balls, bd.fours, bd.sixes, bd.batting_position,
	  mi.inning_number AS inning,
	  0 AS batting_session,
	  CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) = 'bat' THEN 1 ELSE 0 END AS toss,
	  p.player_name,
	  mf.code AS format_code,
	  COALESCE(fd.catches,0) AS catches,
	  COALESCE(fd.run_outs,0) AS run_outs,
	  COALESCE(fd.stumpings,0) AS stumpings,
	  COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
	  (COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
	FROM batting_data bd
	JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
	JOIN match m ON m.match_id = bd.match_id
	LEFT JOIN match_format mf ON mf.id = m.format_id
	LEFT JOIN player p ON p.id = bd.player_id
	-- fielding_data.inning_number + unique (match_id, inning_number, player_id): migration 0095_fielding_data_inning_number.sql
	LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.inning_number = bd.inning_number AND fd.player_id = bd.player_id`

	// When sequence features are enabled, wrap the base query to append extra columns via LATERAL joins.
	if IsSeqEnabled(ctx) {
		seqFields, seqJoins := BuildBattingSeqFragments(ctx)
		if len(seqFields) > 0 {
			q = fmt.Sprintf("SELECT base.*, %s FROM (%s) base %s", strings.Join(seqFields, ", "), q, seqJoins)
		}
	}
	rows, err := db.Pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	baseHeaders := []string{
		"runs", "balls", "fours", "sixes", "batting_position",
		"inning", "batting_session", "toss", "player_name", "format_code",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
	}
	finalHeaders := AppendSeqIfEnabled(ctx, baseHeaders, BattingSeqHeaders())
	out := make([][]string, 0, 2048)
	out = append(out, finalHeaders)
	for rows.Next() {
		vals, err := scanx.ScanToStrings(rows, len(finalHeaders))
		if err != nil {
			return nil, err
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// battingRawStatsLateralSelect is the SELECT list for overall raw stats from feature_raw_stats_snapshots (18 batting columns).
const battingRawStatsLateralSelect = `batting_mean_w3, batting_mean_w5, batting_mean_w10, batting_mean_w20,
	batting_std_w5, batting_std_w10, batting_max_w10, batting_min_w10, batting_median_w10,
	batting_last_1, batting_last_2, batting_last_3,
	batting_career_mean, batting_career_count, batting_pct_zero_w10, batting_trend_w5,
	batting_days_since_last, batting_innings_in_last_90d`

// BattingInferenceRows returns CSV-shaped rows for the batting inference export filtered by format.
// Uses raw windowed stats from feature_raw_stats_snapshots (no form/consistency).
func BattingInferenceRows(ctx context.Context, format string) ([][]string, error) {
	id, err := db.GetGlobalCache().GetFormatID(ctx, strings.ToUpper(strings.TrimSpace(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format_id for %s: %w", format, err)
	}
	formatID := id
	q := `SELECT  
		COALESCE(raw.batting_mean_w3, 0), COALESCE(raw.batting_mean_w5, 0), COALESCE(raw.batting_mean_w10, 0), COALESCE(raw.batting_mean_w20, 0),
		COALESCE(raw.batting_std_w5, 0), COALESCE(raw.batting_std_w10, 0), COALESCE(raw.batting_max_w10, 0), COALESCE(raw.batting_min_w10, 0), COALESCE(raw.batting_median_w10, 0),
		COALESCE(raw.batting_last_1, 0), COALESCE(raw.batting_last_2, 0), COALESCE(raw.batting_last_3, 0),
		COALESCE(raw.batting_career_mean, 0), COALESCE(raw.batting_career_count, 0), COALESCE(raw.batting_pct_zero_w10, 0), COALESCE(raw.batting_trend_w5, 0),
		COALESCE(raw.batting_days_since_last, 0), COALESCE(raw.batting_innings_in_last_90d, 0),
		COALESCE(mi.inning_number, 1) AS batting_inning,
		0 AS batting_session,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		COALESCE(tvv.batting_mean_w5, 0) AS venue,
		COALESCE(tvo.batting_mean_w5, 0) AS opposition,
		p.player_name,
		COALESCE(fd.catches,0) AS catches, COALESCE(fd.run_outs,0) AS run_outs, COALESCE(fd.stumpings,0) AS stumpings, COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		LEFT JOIN match m ON m.match_id = bd.match_id
		LEFT JOIN LATERAL (
		  SELECT ` + battingRawStatsLateralSelect + `
		  FROM feature_raw_stats_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) raw ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_mean_w5 FROM feature_raw_stats_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_mean_w5 FROM feature_raw_stats_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE
		LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.inning_number = bd.inning_number AND fd.player_id = bd.player_id
		WHERE m.format_id = $1`
	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := BattingInferenceHeaders()
	out := make([][]string, 0, 1024)
	out = append(out, headers)
	for rows.Next() {
		vals, err := scanx.ScanToStrings(rows, len(headers))
		if err != nil {
			return nil, err
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// BattingFormatRows returns CSV-shaped rows for the per-format training batting export.
// Uses raw windowed stats from feature_raw_stats_snapshots (no form/consistency).
func BattingFormatRows(ctx context.Context, format string) ([][]string, error) {
	id, err := db.GetGlobalCache().GetFormatID(ctx, strings.ToUpper(strings.TrimSpace(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format_id for %s: %w", format, err)
	}
	formatID := id
	q := `SELECT  
		bd.runs, bd.balls, bd.fours, bd.sixes, bd.batting_position,
		COALESCE(raw.batting_mean_w3, 0), COALESCE(raw.batting_mean_w5, 0), COALESCE(raw.batting_mean_w10, 0), COALESCE(raw.batting_mean_w20, 0),
		COALESCE(raw.batting_std_w5, 0), COALESCE(raw.batting_std_w10, 0), COALESCE(raw.batting_max_w10, 0), COALESCE(raw.batting_min_w10, 0), COALESCE(raw.batting_median_w10, 0),
		COALESCE(raw.batting_last_1, 0), COALESCE(raw.batting_last_2, 0), COALESCE(raw.batting_last_3, 0),
		COALESCE(raw.batting_career_mean, 0), COALESCE(raw.batting_career_count, 0), COALESCE(raw.batting_pct_zero_w10, 0), COALESCE(raw.batting_trend_w5, 0),
		COALESCE(raw.batting_days_since_last, 0), COALESCE(raw.batting_innings_in_last_90d, 0),
		mi.inning_number, 0 AS batting_session,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		COALESCE(tvv.batting_mean_w5, 0) AS batting_venue,
		COALESCE(tvo.batting_mean_w5, 0) AS batting_opposition,
		p.player_name,
		COALESCE(fd.catches,0) AS catches, COALESCE(fd.run_outs,0) AS run_outs, COALESCE(fd.stumpings,0) AS stumpings, COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		LEFT JOIN match m ON m.match_id = bd.match_id
		LEFT JOIN LATERAL (
		  SELECT ` + battingRawStatsLateralSelect + `
		  FROM feature_raw_stats_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) raw ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_mean_w5 FROM feature_raw_stats_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_mean_w5 FROM feature_raw_stats_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE
		LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.inning_number = bd.inning_number AND fd.player_id = bd.player_id
		WHERE m.format_id = $1`
	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := BattingFormatHeaders()
	out := make([][]string, 0, 1024)
	out = append(out, headers)
	for rows.Next() {
		vals, err := scanx.ScanToStrings(rows, len(headers))
		if err != nil {
			return nil, err
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// BattingTrainingRows returns batting export-shaped rows for all matches with match_date < cutoff.
// Used by the backtest training-data API. Form/consistency/venue/opposition are computed on the fly using
// the same EWM and Consistency logic as precompute-features (no fallback to AVG or zero).
func BattingTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
	return battingTrainingRowsImpl(ctx, cutoff, nil)
}

// battingTrainingRowsRawQuery returns SQL and args for the raw batting training query (no snapshot joins).
// If formatIDs is nil, no format filter; otherwise WHERE m.format_id = ANY($1::bigint[]) AND m.match_date < $2.
func battingTrainingRowsRawQuery(formatIDs []int64, cutoff time.Time) (q string, args []any) {
	q = `WITH eligible_matches AS (
		SELECT match_id FROM match m WHERE m.match_date < $1
	),
	innings_sums AS (
		SELECT bd.match_id, bd.inning_number, SUM(bd.runs)::bigint AS total_runs
		FROM batting_data bd
		WHERE bd.match_id IN (SELECT match_id FROM eligible_matches)
		GROUP BY bd.match_id, bd.inning_number
	)
	SELECT
		m.match_date,
		bd.player_id,
		m.format_id,
		COALESCE(m.venue_id, 0),
		COALESCE(mi.bowling_team_opposition_id, 0),
		bd.runs,
		COALESCE(ins.total_runs, 0)::bigint AS innings_runs,
		bd.balls,
		bd.fours,
		bd.sixes,
		bd.batting_position,
		COALESCE(mi.inning_number, 1),
		0 AS batting_session,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		p.player_name,
		COALESCE(fd.catches,0) AS catches, COALESCE(fd.run_outs,0) AS run_outs, COALESCE(fd.stumpings,0) AS stumpings, COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
	FROM batting_data bd
	LEFT JOIN innings_sums ins ON ins.match_id = bd.match_id AND ins.inning_number = bd.inning_number
	LEFT JOIN player p ON bd.player_id = p.id
	LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
	LEFT JOIN match m ON m.match_id = bd.match_id
	LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.inning_number = bd.inning_number AND fd.player_id = bd.player_id
	WHERE m.match_date < $1
	ORDER BY m.match_date ASC, bd.match_id, bd.player_id`
	args = []any{cutoff}
	if formatIDs != nil {
		q = strings.Replace(
			q,
			"WHERE m.match_date < $1",
			"WHERE m.format_id = ANY($1::bigint[]) AND m.match_date < $2",
			2, // replace in eligible_matches and main WHERE
		)
		args = []any{formatIDs, cutoff}
	}
	return q, args
}

// battingTrainingRowRaw holds one scanned row before snapshot computation (for batch N+1 fix).
type battingTrainingRowRaw struct {
	matchDate    time.Time
	playerID     int64
	formatID     int64
	venueID      int64
	oppositionID int64
	runs         string
	inningsRuns  string
	balls        string
	fours        string
	sixes        string
	pos          string
	temp         string
	wind         string
	rain         string
	humidity     string
	cloud        string
	pressure     string
	viscosity    string
	inning       string
	sess         string
	toss         string
	playerName   string
	catches      string
	runOuts      string
	stumpings    string
	runoutsDH    string
	fieldingInv  string
}

func battingTrainingRowsImpl(ctx context.Context, cutoff time.Time, formatIDs []int64) ([][]string, error) {
	q, args := battingTrainingRowsRawQuery(formatIDs, cutoff)
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rawRows []battingTrainingRowRaw
	for rows.Next() {
		var r battingTrainingRowRaw
		err := rows.Scan(
			&r.matchDate, &r.playerID, &r.formatID, &r.venueID, &r.oppositionID,
			&r.runs, &r.inningsRuns, &r.balls, &r.fours, &r.sixes, &r.pos,
			&r.temp, &r.wind, &r.rain, &r.humidity, &r.cloud, &r.pressure, &r.viscosity,
			&r.inning, &r.sess, &r.toss, &r.playerName,
			&r.catches, &r.runOuts, &r.stumpings, &r.runoutsDH, &r.fieldingInv,
		)
		if err != nil {
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Batch-fetch histories to avoid N+1: one query per unique (playerID, matchDate, formatID) and per venue/opposition scope.
	// Use UnixNano for map keys to avoid time.Time equality issues (monotonic clock); convert back to time.Time for db.HistQueryKey.
	type mainKey struct {
		P int64
		T int64 // matchDate.UnixNano()
		F int64
	}
	type venueKey struct {
		P int64
		T int64
		F int64
		V int64
	}
	type oppKey struct {
		P int64
		T int64
		F int64
		O int64
	}
	mainKeys := make(map[mainKey]struct{})
	venueKeys := make(map[venueKey]struct{})
	oppKeys := make(map[oppKey]struct{})
	for _, r := range rawRows {
		tn := r.matchDate.UnixNano()
		mainKeys[mainKey{r.playerID, tn, r.formatID}] = struct{}{}
		if r.venueID != 0 {
			venueKeys[venueKey{r.playerID, tn, r.formatID, r.venueID}] = struct{}{}
		}
		if r.oppositionID != 0 {
			oppKeys[oppKey{r.playerID, tn, r.formatID, r.oppositionID}] = struct{}{}
		}
	}
	// Bulk fetch to avoid N+1
	bulkKeys := make([]db.HistQueryKey, 0, len(mainKeys)+len(venueKeys)+len(oppKeys))
	for k := range mainKeys {
		bulkKeys = append(bulkKeys, db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: 0, V: 0})
	}
	for k := range venueKeys {
		bulkKeys = append(bulkKeys, db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: 0, V: k.V})
	}
	for k := range oppKeys {
		bulkKeys = append(bulkKeys, db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: k.O, V: 0})
	}
	bulkRes, err := db.ListBattingBeforeBulk(ctx, bulkKeys)
	if err != nil {
		return nil, fmt.Errorf("list batting before bulk: %w", err)
	}
	mainCache := make(map[mainKey][]db.InnVal)
	for k := range mainKeys {
		mainCache[k] = bulkRes[db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: 0, V: 0}]
	}
	venueCache := make(map[venueKey][]db.InnVal)
	for k := range venueKeys {
		venueCache[k] = bulkRes[db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: 0, V: k.V}]
	}
	oppCache := make(map[oppKey][]db.InnVal)
	for k := range oppKeys {
		oppCache[k] = bulkRes[db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: k.O, V: 0}]
	}

	headers := BattingTrainingHeaders()
	alpha, lastN, windowN, alphaShort, alphaLong, momentumN := GetFeatureExtractionParams()
	out := make([][]string, 0, len(rawRows)+1)
	out = append(out, headers)
	for _, r := range rawRows {
		tn := r.matchDate.UnixNano()
		mk := mainKey{r.playerID, tn, r.formatID}
		mainHist := mainCache[mk]
		var venueHist, oppHist []db.InnVal
		if r.venueID != 0 {
			venueHist = venueCache[venueKey{r.playerID, tn, r.formatID, r.venueID}]
		}
		if r.oppositionID != 0 {
			oppHist = oppCache[oppKey{r.playerID, tn, r.formatID, r.oppositionID}]
		}
		snap := computeBattingSnapshotFromHistories(
			mainHist,
			venueHist,
			oppHist,
			r.matchDate,
			alpha,
			alphaShort,
			alphaLong,
			lastN,
			windowN,
			momentumN,
		)
		rawStrs := rawStatsToExportStrings(snap.raw)
		row := make([]string, 0, len(headers))
		row = append(row, r.runs, r.inningsRuns, r.balls, r.fours, r.sixes, r.pos)
		row = append(row, rawStrs...)
		row = append(
			row,
			r.temp,
			r.wind,
			r.rain,
			r.humidity,
			r.cloud,
			r.pressure,
			r.viscosity,
			r.inning,
			r.sess,
			r.toss,
			floatToExport(snap.venue),
			floatToExport(snap.opposition),
			cyclicalMonthSin(r.matchDate),
			cyclicalMonthCos(r.matchDate),
			cyclicalDowSin(r.matchDate),
			cyclicalDowCos(r.matchDate),
			r.playerName,
			r.catches,
			r.runOuts,
			r.stumpings,
			r.runoutsDH,
			r.fieldingInv,
			r.matchDate.Format("2006-01-02"),
		)
		out = append(out, row)
	}
	return out, nil
}

func floatToExport(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// BattingTrainingRowsWithFormat returns batting export-shaped rows for matches with match_date < cutoff
// and format_id in the given format's bucket (T20/T20I share a bucket). Form/consistency/venue/opposition
// are computed on the fly using the same EWM and Consistency logic as precompute-features.
func BattingTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, fmt.Errorf("resolve format_id(s) for %s: %w", format, err)
	}
	return battingTrainingRowsImpl(ctx, cutoff, formatIDs)
}

// battingHoldoutRawQuery returns SQL and args for raw batting rows for the given match IDs (same columns as training).
func battingHoldoutRawQuery(matchIDs []int64) (string, []any) {
	q := "WITH " + inningsRunsHoldoutCTE("innings_sums") + `
	SELECT
		m.match_date,
		bd.player_id,
		m.format_id,
		COALESCE(m.venue_id, 0),
		COALESCE(mi.bowling_team_opposition_id, 0),
		bd.runs,
		COALESCE(ins.total_runs, 0)::bigint AS innings_runs,
		bd.balls,
		bd.fours,
		bd.sixes,
		bd.batting_position,
		COALESCE(mi.inning_number, 1),
		0 AS batting_session,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		p.player_name,
		COALESCE(fd.catches,0) AS catches, COALESCE(fd.run_outs,0) AS run_outs, COALESCE(fd.stumpings,0) AS stumpings, COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
	FROM batting_data bd
	LEFT JOIN innings_sums ins ON ins.match_id = bd.match_id AND ins.inning_number = bd.inning_number
	LEFT JOIN player p ON bd.player_id = p.id
	LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
	LEFT JOIN match m ON m.match_id = bd.match_id
	LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.inning_number = bd.inning_number AND fd.player_id = bd.player_id
	WHERE m.match_id = ANY($1::bigint[])`
	return q, []any{matchIDs}
}

// battingHoldoutHeaders returns the CSV header row for batting holdout export (base + raw stats + env context).
func battingHoldoutHeaders() []string {
	baseHeaders := []string{
		"runs", "innings_runs", "balls", "fours", "sixes", "batting_position",
	}
	rawStatsHeaders := features.RawStatsFeatureNamesBatting()
	envContextHeaders := []string{
		"inning", "batting_session", "toss", "batting_venue", "batting_opposition", "player_name",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
		"match_date",
	}
	headers := make([]string, 0, len(baseHeaders)+len(rawStatsHeaders)+len(envContextHeaders))
	headers = append(headers, baseHeaders...)
	headers = append(headers, rawStatsHeaders...)
	headers = append(headers, envContextHeaders...)
	return headers
}

// battingHoldoutRowsImpl returns batting export-shaped rows for the given match IDs with features computed at cutoff.
func battingHoldoutRowsImpl(ctx context.Context, _ []int64, matchIDs []int64, cutoff time.Time) ([][]string, error) {
	if len(matchIDs) == 0 {
		return [][]string{battingHoldoutHeaders()}, nil
	}
	q, args := battingHoldoutRawQuery(matchIDs)
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rawRows []battingTrainingRowRaw
	for rows.Next() {
		var r battingTrainingRowRaw
		if err := rows.Scan(
			&r.matchDate, &r.playerID, &r.formatID, &r.venueID, &r.oppositionID,
			&r.runs, &r.inningsRuns, &r.balls, &r.fours, &r.sixes, &r.pos,
			&r.temp, &r.wind, &r.rain, &r.humidity, &r.cloud, &r.pressure, &r.viscosity,
			&r.inning, &r.sess, &r.toss, &r.playerName,
			&r.catches, &r.runOuts, &r.stumpings, &r.runoutsDH, &r.fieldingInv,
		); err != nil {
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Build keys using cutoff (not row matchDate) so features are "as of cutoff"
	cutoffNano := cutoff.UnixNano()
	type mainKey struct {
		P, T int64
		F    int64
	}
	type venueKey struct {
		P, T int64
		F    int64
		V    int64
	}
	type oppKey struct {
		P, T int64
		F    int64
		O    int64
	}
	mainKeys := make(map[mainKey]struct{})
	venueKeys := make(map[venueKey]struct{})
	oppKeys := make(map[oppKey]struct{})
	for _, r := range rawRows {
		mainKeys[mainKey{r.playerID, cutoffNano, r.formatID}] = struct{}{}
		if r.venueID != 0 {
			venueKeys[venueKey{r.playerID, cutoffNano, r.formatID, r.venueID}] = struct{}{}
		}
		if r.oppositionID != 0 {
			oppKeys[oppKey{r.playerID, cutoffNano, r.formatID, r.oppositionID}] = struct{}{}
		}
	}
	bulkKeys := make([]db.HistQueryKey, 0, len(mainKeys)+len(venueKeys)+len(oppKeys))
	for k := range mainKeys {
		bulkKeys = append(bulkKeys, db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: 0, V: 0})
	}
	for k := range venueKeys {
		bulkKeys = append(bulkKeys, db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: 0, V: k.V})
	}
	for k := range oppKeys {
		bulkKeys = append(bulkKeys, db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: k.O, V: 0})
	}
	bulkRes, err := db.ListBattingBeforeBulk(ctx, bulkKeys)
	if err != nil {
		return nil, fmt.Errorf("list batting before bulk: %w", err)
	}
	mainCache := make(map[mainKey][]db.InnVal)
	for k := range mainKeys {
		mainCache[k] = bulkRes[db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: 0, V: 0}]
	}
	venueCache := make(map[venueKey][]db.InnVal)
	for k := range venueKeys {
		venueCache[k] = bulkRes[db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: 0, V: k.V}]
	}
	oppCache := make(map[oppKey][]db.InnVal)
	for k := range oppKeys {
		oppCache[k] = bulkRes[db.HistQueryKey{P: k.P, T: time.Unix(0, k.T), F: k.F, O: k.O, V: 0}]
	}
	alpha, lastN, windowN, alphaShort, alphaLong, momentumN := GetFeatureExtractionParams()
	headers := battingHoldoutHeaders()
	out := make([][]string, 0, len(rawRows)+1)
	out = append(out, headers)
	for _, r := range rawRows {
		mk := mainKey{r.playerID, cutoffNano, r.formatID}
		mainHist := mainCache[mk]
		var venueHist, oppHist []db.InnVal
		if r.venueID != 0 {
			venueHist = venueCache[venueKey{r.playerID, cutoffNano, r.formatID, r.venueID}]
		}
		if r.oppositionID != 0 {
			oppHist = oppCache[oppKey{r.playerID, cutoffNano, r.formatID, r.oppositionID}]
		}
		snap := computeBattingSnapshotFromHistories(
			mainHist,
			venueHist,
			oppHist,
			cutoff,
			alpha,
			alphaShort,
			alphaLong,
			lastN,
			windowN,
			momentumN,
		)
		rawStrs := rawStatsToExportStrings(snap.raw)
		row := make([]string, 0, len(headers))
		row = append(row, r.runs, r.inningsRuns, r.balls, r.fours, r.sixes, r.pos)
		row = append(row, rawStrs...)
		row = append(
			row,
			r.temp,
			r.wind,
			r.rain,
			r.humidity,
			r.cloud,
			r.pressure,
			r.viscosity,
			r.inning,
			r.sess,
			r.toss,
			floatToExport(snap.venue),
			floatToExport(snap.opposition),
			cyclicalMonthSin(r.matchDate),
			cyclicalMonthCos(r.matchDate),
			cyclicalDowSin(r.matchDate),
			cyclicalDowCos(r.matchDate),
			r.playerName,
			r.catches,
			r.runOuts,
			r.stumpings,
			r.runoutsDH,
			r.fieldingInv,
			r.matchDate.Format("2006-01-02"),
		)
		out = append(out, row)
	}
	return out, nil
}

// BattingHoldoutRows returns batting export-shaped rows for the next limit matches after cutoff (walk-forward holdout).
// Features (form, consistency, venue, opposition) are computed at cutoff so predictions are temporally valid.
func BattingHoldoutRows(ctx context.Context, format string, cutoff time.Time, limit int) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, fmt.Errorf("resolve format_id(s) for %s: %w", format, err)
	}
	matchList, err := db.ListMatchIDsAfter(ctx, formatIDs, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("list match IDs after: %w", err)
	}
	matchIDs := make([]int64, 0, len(matchList))
	for _, it := range matchList {
		matchIDs = append(matchIDs, it.MatchID)
	}
	return battingHoldoutRowsImpl(ctx, formatIDs, matchIDs, cutoff)
}
