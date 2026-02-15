package exportqueries

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/scanx"
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
		"inning", "batting_session", "toss", "season_id", "player_name", "format_code",
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
		"batting_venue", "batting_opposition", "season_id", "player_name",
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
// Used by the backtest training-data API. No format filter: features are built from data across formats.
func BattingTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
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
		) tvv ON TRUE LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.player_id = bd.player_id WHERE m.match_date < $1`
	rows, err := db.Pool.Query(ctx, q, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{
		"runs", "balls", "fours", "sixes", "batting_position",
		"batting_consistency", "batting_form", "temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "batting_session", "toss", "batting_venue", "batting_opposition", "season_id", "player_name",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
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

// BattingTrainingRowsWithFormat returns batting export-shaped rows for matches with match_date < cutoff
// and format_id in the given format's bucket (T20/T20I share a bucket). Use when format is a specific code (e.g. T20, ODI).
func BattingTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, fmt.Errorf("resolve format_id(s) for %s: %w", format, err)
	}
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
		) tvv ON TRUE LEFT JOIN fielding_data fd ON fd.match_id = bd.match_id AND fd.player_id = bd.player_id WHERE m.format_id = ANY($1::bigint[]) AND m.match_date < $2`
	rows, err := db.Pool.Query(ctx, q, formatIDs, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{
		"runs", "balls", "fours", "sixes", "batting_position",
		"batting_consistency", "batting_form", "temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "batting_session", "toss", "batting_venue", "batting_opposition", "season_id", "player_name",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
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
