package exportqueries

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/scanx"
)

// BattingUnifiedRows returns CSV-shaped rows for the unified batting export.
// Mirrors legacy exportBattingUnified: base batting row + per-format as-of features and fielding.
func BattingUnifiedRows(ctx context.Context) ([][]string, error) {
	cache := db.GetGlobalCache()
	// Resolve known format ids (some may be missing depending on data)
	ids := make([]any, 0, 4)
	codes := []string{"TEST", "ODI", "T20I", "T20"}
	for _, code := range codes {
		id, err := cache.GetFormatID(ctx, code)
		if err != nil || id <= 0 {
			ids = append(ids, nil)
			continue
		}
		ids = append(ids, id)
	}
	q := `
	SELECT 
	  bd.runs, bd.balls, bd.fours, bd.sixes, bd.batting_position,
	  w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure,
	  CASE WHEN w.viscosity IS NULL THEN 0
	       WHEN lower(w.viscosity)='dry' THEN 0
	       WHEN lower(w.viscosity)='humid' THEN 1
	       WHEN lower(w.viscosity)='windy' THEN 2
	       ELSE 0 END AS viscosity,
	  mi.inning_number AS inning,
	  0 AS batting_session,
	  CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) = 'bat' THEN 1 ELSE 0 END AS toss,
	  s.id AS season_id,
	  EXTRACT(EPOCH FROM m.match_date)::bigint AS match_date_unix,
	  p.player_name,
	  mf.code AS format_code,
	  COALESCE(fd.catches,0) AS catches,
	  COALESCE(fd.run_outs,0) AS run_outs,
	  COALESCE(fd.stumpings,0) AS stumpings,
	  COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
	  (COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements,
	  -- TEST as-of
	  tf.bat_form   AS bat_form_TEST_asof,
	  tc.bat_consistency AS bat_consistency_TEST_asof,
	  tvo.bat_value AS bat_vs_opp_TEST_asof,
	  tvv.bat_value AS bat_at_venue_TEST_asof,
	  -- ODI as-of
	  of.bat_form   AS bat_form_ODI_asof,
	  oc.bat_consistency AS bat_consistency_ODI_asof,
	  ovo.bat_value AS bat_vs_opp_ODI_asof,
	  ovv.bat_value AS bat_at_venue_ODI_asof,
	  -- T20I as-of
	  iif.bat_form   AS bat_form_T20I_asof,
	  iic.bat_consistency AS bat_consistency_T20I_asof,
	  iivo.bat_value AS bat_vs_opp_T20I_asof,
	  iivv.bat_value AS bat_at_venue_T20I_asof,
	  -- T20 as-of
	  t20f.bat_form   AS bat_form_T20_asof,
	  t20c.bat_consistency AS bat_consistency_T20_asof,
	  t20vo.bat_value AS bat_vs_opp_T20_asof,
	  t20vv.bat_value AS bat_at_venue_T20_asof
	FROM batting_data bd
	JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
	JOIN match m ON m.match_id = bd.match_id
	LEFT JOIN match_format mf ON mf.id = m.format_id
	LEFT JOIN player p ON p.id = bd.player_id
	LEFT JOIN season s ON s.id = m.season_id
	LEFT JOIN (
	  SELECT * FROM weather_data WHERE session='batting'
	) w ON w.match_id = bd.match_id
	LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.player_id = bd.player_id
	-- TEST lateral joins
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $1 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) tf ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
	  WHERE player_id=bd.player_id AND format_id = $1 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) tc ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $1 AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) tvo ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $1 AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) tvv ON TRUE
	-- ODI lateral joins
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $2 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) of ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
	  WHERE player_id=bd.player_id AND format_id = $2 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) oc ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $2 AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) ovo ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $2 AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) ovv ON TRUE
	-- T20I lateral joins
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $3 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) iif ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
	  WHERE player_id=bd.player_id AND format_id = $3 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) iic ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $3 AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) iivo ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $3 AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) iivv ON TRUE
	-- T20 lateral joins
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $4 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) t20f ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
	  WHERE player_id=bd.player_id AND format_id = $4 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) t20c ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $4 AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) t20vo ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $4 AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
	) t20vv ON TRUE`

	// When sequence features are enabled, wrap the base query to append extra columns via LATERAL joins.
	if IsSeqEnabled(ctx) {
		seqFields, seqJoins := BuildBattingSeqFragments(ctx)
		if len(seqFields) > 0 {
			q = fmt.Sprintf("SELECT base.*, %s FROM (%s) base %s", strings.Join(seqFields, ", "), q, seqJoins)
		}
	}
	rows, err := db.Pool.Query(ctx, q, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	baseHeaders := []string{
		"runs", "balls", "fours", "sixes", "batting_position",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "batting_session", "toss", "season_id", "match_date_unix", "player_name", "format_code",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
		"bat_form_TEST_asof", "bat_consistency_TEST_asof", "bat_vs_opp_TEST_asof", "bat_at_venue_TEST_asof",
		"bat_form_ODI_asof", "bat_consistency_ODI_asof", "bat_vs_opp_ODI_asof", "bat_at_venue_ODI_asof",
		"bat_form_T20I_asof", "bat_consistency_T20I_asof", "bat_vs_opp_T20I_asof", "bat_at_venue_T20I_asof",
		"bat_form_T20_asof", "bat_consistency_T20_asof", "bat_vs_opp_T20_asof", "bat_at_venue_T20_asof",
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

// BattingLegacyRows returns CSV-shaped rows for the legacy (combined) batting export.
// It mirrors the SELECT and header order from cmd/export-dataset:exportBatting.
func BattingLegacyRows(ctx context.Context) ([][]string, error) {
	const q = `SELECT  
		bd.runs,
		bd.balls,
		bd.fours,
		bd.sixes,
		bd.batting_position,
		tc.batting_consistency,
		tf.batting_form,
		w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure,
		CASE 
			WHEN w.viscosity IS NULL THEN 0
			WHEN lower(w.viscosity) = 'dry' THEN 0
			WHEN lower(w.viscosity) = 'humid' THEN 1
			WHEN lower(w.viscosity) = 'windy' THEN 2
			ELSE 0
		END AS viscosity,
		mi.inning_number,
		0 AS batting_session,
		CASE 
			WHEN m.toss_decision IS NULL THEN 0
			WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1
			ELSE 0
		END AS toss,
		tvv.batting_venue,
		tvo.batting_opposition,
		s.id AS season_id,
		EXTRACT(EPOCH FROM m.match_date)::bigint AS match_date_unix,
		p.player_name
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'batting'
		) w ON bd.match_id = w.match_id
		LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		LEFT JOIN match m ON m.match_id = bd.match_id
		LEFT JOIN venue v ON v.id = m.venue_id
		LEFT JOIN opposition o ON o.id = mi.bowling_team_opposition_id
		LEFT JOIN season s ON s.id = m.season_id
		-- Latest overall batting form as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_form
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		-- Latest overall batting consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_consistency
		  FROM feature_consistency_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest batting form vs opposition as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_opposition
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		-- Latest batting form at venue as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_venue
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE`

	rows, err := db.Pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"runs", "balls", "fours", "sixes", "batting_position", "batting_consistency", "batting_form",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity", "inning", "batting_session", "toss",
		"batting_venue", "batting_opposition", "season_id", "match_date_unix", "player_name",
	}
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

// BattingInferenceRows returns CSV-shaped rows for the batting inference export filtered by format.
// Mirrors legacy exportBattingFormatInference headers and order.
func BattingInferenceRows(ctx context.Context, format string) ([][]string, error) {
	id, err := db.GetGlobalCache().GetFormatID(ctx, strings.ToUpper(strings.TrimSpace(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format_id for %s: %w", format, err)
	}
	formatID := id
	q := `SELECT  
		COALESCE(tc.batting_consistency, 0) AS batting_consistency,
		COALESCE(tf.batting_form, 0) AS batting_form,
		COALESCE(w.temp, 0) AS batting_temp,
		COALESCE(w.wind, 0) AS batting_wind,
		COALESCE(w.rain, 0) AS batting_rain,
		COALESCE(w.humidity, 0) AS batting_humidity,
		COALESCE(w.cloud, 0) AS batting_cloud,
		COALESCE(w.pressure, 0) AS batting_pressure,
		CASE 
			WHEN w.viscosity IS NULL THEN 0
			WHEN lower(w.viscosity) = 'humid' THEN 1
			ELSE 0
		END AS batting_viscosity,
		COALESCE(mi.inning_number, 1) AS batting_inning,
		0 AS batting_session,
		CASE 
			WHEN m.toss_decision IS NULL THEN 0
			WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1
			ELSE 0
		END AS toss,
		COALESCE(tvv.batting_venue, 0) AS venue,
		COALESCE(tvo.batting_opposition, 0) AS opposition,
		COALESCE(s.id, 0) AS season,
		EXTRACT(EPOCH FROM m.match_date)::bigint AS match_date_unix,
		p.player_name,
		COALESCE(fd.catches,0) AS catches,
		COALESCE(fd.run_outs,0) AS run_outs,
		COALESCE(fd.stumpings,0) AS stumpings,
		COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'batting'
		) w ON bd.match_id = w.match_id
		LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		LEFT JOIN match m ON m.match_id = bd.match_id
		LEFT JOIN season s ON s.id = m.season_id
		-- Latest overall batting form and consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_form
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_consistency
		  FROM feature_consistency_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest batting form vs opposition and at venue
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_opposition
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_venue
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.player_id = bd.player_id WHERE m.format_id = $1`
	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{
		"batting_consistency",
		"batting_form",
		"batting_temp",
		"batting_wind",
		"batting_rain",
		"batting_humidity",
		"batting_cloud",
		"batting_pressure",
		"batting_viscosity",
		"batting_inning",
		"batting_session",
		"toss",
		"venue",
		"opposition",
		"season",
		"match_date_unix",
		"player_name",
		"catches",
		"run_outs",
		"stumpings",
		"runouts_direct_hits",
		"fielding_involvements",
	}
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
// Mirrors legacy exportBattingFormat headers and order.
func BattingFormatRows(ctx context.Context, format string) ([][]string, error) {
	id, err := db.GetGlobalCache().GetFormatID(ctx, strings.ToUpper(strings.TrimSpace(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format_id for %s: %w", format, err)
	}
	formatID := id
	q := `SELECT  
		bd.runs,
		bd.balls,
		bd.fours,
		bd.sixes,
		bd.batting_position,
		tc.batting_consistency,
		tf.batting_form,
		w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure,
		CASE 
			WHEN w.viscosity IS NULL THEN 0
			WHEN lower(w.viscosity) = 'dry' THEN 0
			WHEN lower(w.viscosity) = 'humid' THEN 1
			WHEN lower(w.viscosity) = 'windy' THEN 2
			ELSE 0
		END AS viscosity,
		mi.inning_number,
		0 AS batting_session,
		CASE 
			WHEN m.toss_decision IS NULL THEN 0
			WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1
			ELSE 0
		END AS toss,
		tvv.batting_venue,
		tvo.batting_opposition,
		s.id AS season_id,
		EXTRACT(EPOCH FROM m.match_date)::bigint AS match_date_unix,
		p.player_name,
		COALESCE(fd.catches,0) AS catches,
		COALESCE(fd.run_outs,0) AS run_outs,
		COALESCE(fd.stumpings,0) AS stumpings,
		COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'batting'
		) w ON bd.match_id = w.match_id
		LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		LEFT JOIN match m ON m.match_id = bd.match_id
		LEFT JOIN season s ON s.id = m.season_id
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_form, n_samples_bat FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_consistency, n_samples_bat FROM feature_consistency_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_opposition, n_samples_bat AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='opposition' AND scope_id = mi.bowling_team_opposition_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_venue, n_samples_bat AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = m.format_id AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.player_id = bd.player_id WHERE m.format_id = $1`
	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{
		"runs",
		"balls",
		"fours",
		"sixes",
		"batting_position",
		"batting_consistency",
		"batting_form",
		"temp",
		"wind",
		"rain",
		"humidity",
		"cloud",
		"pressure",
		"viscosity",
		"inning",
		"batting_session",
		"toss",
		"batting_venue",
		"batting_opposition",
		"season_id",
		"match_date_unix",
		"player_name",
		"catches",
		"run_outs",
		"stumpings",
		"runouts_direct_hits",
		"fielding_involvements",
	}
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
		COALESCE(w.temp, 0), COALESCE(w.wind, 0), COALESCE(w.rain, 0), COALESCE(w.humidity, 0), COALESCE(w.cloud, 0), COALESCE(w.pressure, 0),
		CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
		COALESCE(mi.inning_number, 1),
		0 AS batting_session,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		COALESCE(s.id, 0) AS season_id,
		p.player_name,
		COALESCE(fd.catches,0), COALESCE(fd.run_outs,0), COALESCE(fd.stumpings,0), COALESCE(fd.runouts_direct_hits,0),
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
	FROM batting_data bd
	LEFT JOIN innings_sums ins ON ins.match_id = bd.match_id AND ins.inning_number = bd.inning_number
	LEFT JOIN player p ON bd.player_id = p.id
	LEFT JOIN (SELECT * FROM weather_data WHERE session = 'batting') w ON bd.match_id = w.match_id
	LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
	LEFT JOIN match m ON m.match_id = bd.match_id
	LEFT JOIN season s ON s.id = m.season_id
	LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.player_id = bd.player_id
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
	seasonID     string
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
			&r.inning, &r.sess, &r.toss, &r.seasonID, &r.playerName,
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

	headers := []string{
		"runs", "innings_runs", "balls", "fours", "sixes", "batting_position",
		"batting_consistency", "batting_form", "batting_form_short", "batting_form_long", "batting_momentum",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "batting_session", "toss", "batting_venue", "batting_opposition", "season_id", "player_name",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
		"match_date",
	}
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
		row := []string{
			r.runs, r.inningsRuns, r.balls, r.fours, r.sixes, r.pos,
			floatToExport(
				snap.consistency,
			), floatToExport(snap.form), floatToExport(snap.formShort), floatToExport(snap.formLong), floatToExport(snap.momentum),
			r.temp, r.wind, r.rain, r.humidity, r.cloud, r.pressure, r.viscosity,
			r.inning, r.sess, r.toss,
			floatToExport(snap.venue), floatToExport(snap.opposition),
			r.seasonID, r.playerName,
			r.catches, r.runOuts, r.stumpings, r.runoutsDH, r.fieldingInv,
			r.matchDate.Format("2006-01-02"),
		}
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
		COALESCE(w.temp, 0), COALESCE(w.wind, 0), COALESCE(w.rain, 0), COALESCE(w.humidity, 0), COALESCE(w.cloud, 0), COALESCE(w.pressure, 0),
		CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
		COALESCE(mi.inning_number, 1),
		0 AS batting_session,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		COALESCE(s.id, 0) AS season_id,
		p.player_name,
		COALESCE(fd.catches,0), COALESCE(fd.run_outs,0), COALESCE(fd.stumpings,0), COALESCE(fd.runouts_direct_hits,0),
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
	FROM batting_data bd
	LEFT JOIN innings_sums ins ON ins.match_id = bd.match_id AND ins.inning_number = bd.inning_number
	LEFT JOIN player p ON bd.player_id = p.id
	LEFT JOIN (SELECT * FROM weather_data WHERE session = 'batting') w ON bd.match_id = w.match_id
	LEFT JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
	LEFT JOIN match m ON m.match_id = bd.match_id
	LEFT JOIN season s ON s.id = m.season_id
	LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.player_id = bd.player_id
	WHERE m.match_id = ANY($1::bigint[])`
	return q, []any{matchIDs}
}

// battingHoldoutRowsImpl returns batting export-shaped rows for the given match IDs with features computed at cutoff.
func battingHoldoutRowsImpl(ctx context.Context, _ []int64, matchIDs []int64, cutoff time.Time) ([][]string, error) {
	if len(matchIDs) == 0 {
		headers := []string{
			"runs", "innings_runs", "balls", "fours", "sixes", "batting_position",
			"batting_consistency", "batting_form", "batting_form_short", "batting_form_long", "batting_momentum",
			"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
			"inning", "batting_session", "toss", "batting_venue", "batting_opposition", "season_id", "player_name",
			"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
			"match_date",
		}
		return [][]string{headers}, nil
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
			&r.inning, &r.sess, &r.toss, &r.seasonID, &r.playerName,
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
	headers := []string{
		"runs", "innings_runs", "balls", "fours", "sixes", "batting_position",
		"batting_consistency", "batting_form", "batting_form_short", "batting_form_long", "batting_momentum",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "batting_session", "toss", "batting_venue", "batting_opposition", "season_id", "player_name",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
		"match_date",
	}
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
		row := []string{
			r.runs, r.inningsRuns, r.balls, r.fours, r.sixes, r.pos,
			floatToExport(
				snap.consistency,
			), floatToExport(snap.form), floatToExport(snap.formShort), floatToExport(snap.formLong), floatToExport(snap.momentum),
			r.temp, r.wind, r.rain, r.humidity, r.cloud, r.pressure, r.viscosity,
			r.inning, r.sess, r.toss,
			floatToExport(snap.venue), floatToExport(snap.opposition),
			r.seasonID, r.playerName,
			r.catches, r.runOuts, r.stumpings, r.runoutsDH, r.fieldingInv,
			r.matchDate.Format("2006-01-02"),
		}
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
