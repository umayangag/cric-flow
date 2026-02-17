package exportqueries

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/scanx"
)

// BowlingUnifiedRows returns CSV-shaped rows for the unified bowling export.
// Mirrors legacy exportBowlingUnified: base bowling row + per-format as-of features and fielding.
func BowlingUnifiedRows(ctx context.Context) ([][]string, error) {
	cache := db.GetGlobalCache()
	// Resolve known format ids similar to legacy
	ids := make([]any, 0, 4)
	codes := []string{"TEST", "ODI", "T20I", "T20"}
	for _, code := range codes {
		id, err := cache.GetFormatID(ctx, code)
		if err != nil || id <= 0 {
			ids = append(ids, int64(0))
			continue
		}
		ids = append(ids, id)
	}
	q := `
	SELECT 
	  bw.overs, bw.balls, bw.maidens, bw.runs, bw.wickets, bw.dots, bw.fours, bw.sixes, bw.econ, bw.wides, bw.no_balls,
	  w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure,
	  CASE WHEN w.viscosity IS NULL THEN 0
	       WHEN lower(w.viscosity)='dry' THEN 0
	       WHEN lower(w.viscosity)='humid' THEN 1
	       WHEN lower(w.viscosity)='windy' THEN 2
	       ELSE 0 END AS viscosity,
	  mi.inning_number AS inning,
	  0 AS bowling_session,
	  CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) = 'bat' THEN 1 ELSE 0 END AS toss,
	  s.id AS season_id,
	  p.player_name,
	  mf.code AS format_code,
	  COALESCE(fd.catches,0) AS catches,
	  COALESCE(fd.run_outs,0) AS run_outs,
	  COALESCE(fd.stumpings,0) AS stumpings,
	  COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
	  (COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements,
	   -- TEST
	   tf.bowl_form   AS bowl_form_TEST_asof,
	   tc.bowl_consistency AS bowl_consistency_TEST_asof,
	   tvo.bowl_value AS bowl_vs_opp_TEST_asof,
	   tvv.bowl_value AS bowl_at_venue_TEST_asof,
	   -- ODI
	   of.bowl_form   AS bowl_form_ODI_asof,
	   oc.bowl_consistency AS bowl_consistency_ODI_asof,
	   ovo.bowl_value AS bowl_vs_opp_ODI_asof,
	   ovv.bowl_value AS bowl_at_venue_ODI_asof,
	   -- T20I
	   iif.bowl_form   AS bowl_form_T20I_asof,
	   iic.bowl_consistency AS bowl_consistency_T20I_asof,
	   iivo.bowl_value AS bowl_vs_opp_T20I_asof,
	   iivv.bowl_value AS bowl_at_venue_T20I_asof,
	   -- T20
	   t20f.bowl_form   AS bowl_form_T20_asof,
	   t20c.bowl_consistency AS bowl_consistency_T20_asof,
	   t20vo.bowl_value AS bowl_vs_opp_T20_asof,
	   t20vv.bowl_value AS bowl_at_venue_T20_asof
	 FROM bowling_data bw
		JOIN match_inning mi ON mi.match_id = bw.match_id AND mi.inning_number = bw.inning_number
		JOIN match m ON m.match_id = bw.match_id
		LEFT JOIN match_format mf ON mf.id = m.format_id
		LEFT JOIN player p ON p.id = bw.player_id
		LEFT JOIN season s ON s.id = m.season_id
		LEFT JOIN (
		  SELECT * FROM weather_data WHERE session='bowling'
		) w ON w.match_id = bw.match_id
		LEFT JOIN fielding_data fd ON fd.match_id = bw.match_id AND fd.player_id = bw.player_id
		-- TEST laterals
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $1 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=bw.player_id AND format_id = $1 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $1 AND scope='opposition' AND scope_id = mi.batting_team_opposition_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $1 AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE
		-- ODI laterals
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $2 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) of ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=bw.player_id AND format_id = $2 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) oc ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $2 AND scope='opposition' AND scope_id = mi.batting_team_opposition_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) ovo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $2 AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) ovv ON TRUE
		-- T20I laterals
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $3 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) iif ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=bw.player_id AND format_id = $3 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) iic ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $3 AND scope='opposition' AND scope_id = mi.batting_team_opposition_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) iivo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $3 AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) iivv ON TRUE
		-- T20 laterals
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $4 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) t20f ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=bw.player_id AND format_id = $4 AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) t20c ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $4 AND scope='opposition' AND scope_id = mi.batting_team_opposition_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) t20vo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $4 AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date ORDER BY as_of_date DESC LIMIT 1
		) t20vv ON TRUE`
	// When sequence features are enabled, wrap the base query to append extra columns via LATERAL joins.
	if IsSeqEnabled(ctx) {
		seqFields, seqJoins := BuildBowlingSeqFragments(ctx)
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
		"overs", "balls", "maidens", "runs", "wickets", "dots", "fours", "sixes", "econ", "wides", "no_balls",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "bowling_session", "toss", "season_id", "player_name", "format_code",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
		"bowl_form_TEST_asof", "bowl_consistency_TEST_asof", "bowl_vs_opp_TEST_asof", "bowl_at_venue_TEST_asof",
		"bowl_form_ODI_asof", "bowl_consistency_ODI_asof", "bowl_vs_opp_ODI_asof", "bowl_at_venue_ODI_asof",
		"bowl_form_T20I_asof", "bowl_consistency_T20I_asof", "bowl_vs_opp_T20I_asof", "bowl_at_venue_T20I_asof",
		"bowl_form_T20_asof", "bowl_consistency_T20_asof", "bowl_vs_opp_T20_asof", "bowl_at_venue_T20_asof",
	}
	finalHeaders := AppendSeqIfEnabled(ctx, baseHeaders, BowlingSeqHeaders())
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

// BowlingLegacyRows returns CSV-shaped rows for the legacy (combined) bowling export.
// It mirrors the SELECT and header order from cmd/export-dataset:exportBowling.
func BowlingLegacyRows(ctx context.Context) ([][]string, error) {
	const q = `SELECT  
		b.runs,
		b.balls,
		b.wickets,
		tc.bowling_consistency,
		tf.bowling_form,
		w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure, w.viscosity,
		mi.inning_number,
		NULL,
		m.toss_decision,
		tvv.bowling_venue,
		tvo.bowling_opposition,
		s.id AS season_id,
		p.player_name
		FROM bowling_data b
		LEFT JOIN player p ON b.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'bowling'
		) w ON b.match_id = w.match_id
		LEFT JOIN match_inning mi ON mi.match_id = b.match_id AND mi.inning_number = b.inning_number
		LEFT JOIN match m ON m.match_id = b.match_id
		LEFT JOIN venue v ON v.id = m.venue_id
		LEFT JOIN opposition o ON o.id = mi.batting_team_opposition_id
		LEFT JOIN season s ON s.id = m.season_id
		-- Latest overall bowling form as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_form
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		-- Latest overall bowling consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_consistency
		  FROM feature_consistency_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest bowling form vs opposition as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_opposition
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='opposition' AND scope_id = mi.batting_team_opposition_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		-- Latest bowling form at venue as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_venue
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE`

	rows, err := db.Pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"runs", "balls", "wickets", "bowling_consistency", "bowling_form",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "bowling_session", "toss", "bowling_venue", "bowling_opposition", "season_id", "player_name",
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

// BowlingInferenceRows returns CSV-shaped rows for the bowling inference export filtered by format.
// Mirrors legacy exportBowlingFormatInference headers and order.
func BowlingInferenceRows(ctx context.Context, format string) ([][]string, error) {
	id, err := db.GetGlobalCache().GetFormatID(ctx, strings.ToUpper(strings.TrimSpace(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format_id for %s: %w", format, err)
	}
	formatID := id
	q := `SELECT  
		COALESCE(tc.bowling_consistency, 0) AS bowling_consistency,
		COALESCE(tf.bowling_form, 0) AS bowling_form,
		COALESCE(w.temp, 0) AS bowling_temp,
		COALESCE(w.wind, 0) AS bowling_wind,
		COALESCE(w.rain, 0) AS bowling_rain,
		COALESCE(w.humidity, 0) AS bowling_humidity,
		COALESCE(w.cloud, 0) AS bowling_cloud,
		COALESCE(w.pressure, 0) AS bowling_pressure,
		CASE 
			WHEN w.viscosity IS NULL THEN 0
			WHEN lower(w.viscosity) = 'humid' THEN 1
			ELSE 0
		END AS bowling_viscosity,
		COALESCE(mi.inning_number, 1) AS batting_inning,
		0 AS bowling_session,
		CASE 
			WHEN m.toss_decision IS NULL THEN 0
			WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1
			ELSE 0
		END AS toss,
		COALESCE(tvv.bowling_venue, 0) AS bowling_venue,
		COALESCE(tvo.bowling_opposition, 0) AS bowling_opposition,
		COALESCE(s.id, 0) AS season,
		p.player_name,
		COALESCE(fd.catches,0) AS catches,
		COALESCE(fd.run_outs,0) AS run_outs,
		COALESCE(fd.stumpings,0) AS stumpings,
		COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
		FROM bowling_data b
		LEFT JOIN player p ON b.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'bowling'
		) w ON b.match_id = w.match_id
		LEFT JOIN match_inning mi ON mi.match_id = b.match_id AND mi.inning_number = b.inning_number
		LEFT JOIN match m ON m.match_id = b.match_id
		LEFT JOIN season s ON s.id = m.season_id
		-- Latest overall bowling form and consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_form
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_consistency
		  FROM feature_consistency_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest bowling form vs opposition and at venue
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_opposition
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='opposition' AND scope_id = mi.batting_team_opposition_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_venue
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE LEFT JOIN fielding_data fd ON fd.match_id = b.match_id AND fd.player_id = b.player_id WHERE m.format_id = $1`
	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{
		"bowling_consistency",
		"bowling_form",
		"bowling_temp",
		"bowling_wind",
		"bowling_rain",
		"bowling_humidity",
		"bowling_cloud",
		"bowling_pressure",
		"bowling_viscosity",
		"batting_inning",
		"bowling_session",
		"toss",
		"bowling_venue",
		"bowling_opposition",
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

// BowlingFormatRows returns CSV-shaped rows for the per-format training bowling export.
// Mirrors legacy exportBowlingFormat headers and order (including a trailing format_code column).
func BowlingFormatRows(ctx context.Context, format string) ([][]string, error) {
	id, err := db.GetGlobalCache().GetFormatID(ctx, strings.ToUpper(strings.TrimSpace(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format_id for %s: %w", format, err)
	}
	formatID := id
	q := `SELECT  
		b.runs,
		b.balls,
		b.wickets,
		tc.bowling_consistency,
		tf.bowling_form,
		w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure, w.viscosity,
		mi.inning_number,
		NULL,
		m.toss_decision,
		tvv.bowling_venue,
		tvo.bowling_opposition,
		s.id AS season_id,
		p.player_name,
		COALESCE(fd.catches,0) AS catches,
		COALESCE(fd.run_outs,0) AS run_outs,
		COALESCE(fd.stumpings,0) AS stumpings,
		COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements
		FROM bowling_data b
		LEFT JOIN player p ON b.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'bowling'
		) w ON b.match_id = w.match_id
		LEFT JOIN match_inning mi ON mi.match_id = b.match_id AND mi.inning_number = b.inning_number
		LEFT JOIN match m ON m.match_id = b.match_id
		LEFT JOIN season s ON s.id = m.season_id
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_opposition, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='opposition' AND scope_id = mi.batting_team_opposition_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_venue, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = m.format_id AND scope='venue' AND scope_id = m.venue_id AND as_of_date <= m.match_date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE 
		LEFT JOIN fielding_data fd ON fd.match_id = b.match_id AND fd.player_id = b.player_id 
		WHERE m.format_id = $1`
	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{
		"runs",
		"balls",
		"wickets",
		"bowling_consistency",
		"bowling_form",
		"temp",
		"wind",
		"rain",
		"humidity",
		"cloud",
		"pressure",
		"viscosity",
		"inning",
		"bowling_session",
		"toss",
		"bowling_venue",
		"bowling_opposition",
		"season_id",
		"player_name",
		"catches",
		"run_outs",
		"stumpings",
		"runouts_direct_hits",
		"fielding_involvements",
		"format_code",
	}
	out := make([][]string, 0, 1024)
	out = append(out, headers)
	fmtcode := strings.ToUpper(strings.TrimSpace(format))
	for rows.Next() {
		vals, err := scanx.ScanToStrings(rows, len(headers)-1)
		if err != nil {
			return nil, err
		}
		vals = append(vals, fmtcode)
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// BowlingTrainingRows returns bowling export-shaped rows for all matches with match_date < cutoff.
// Used by the backtest training-data API. Form/consistency/venue/opposition are computed on the fly using
// the same EWM and Consistency logic as precompute-features (no fallback to AVG or zero).
func BowlingTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
	return bowlingTrainingRowsImpl(ctx, cutoff, nil)
}

// bowlingTrainingRowsRawQuery returns SQL and args for the raw bowling training query (no snapshot joins).
func bowlingTrainingRowsRawQuery(formatIDs []int64, cutoff time.Time) (q string, args []any) {
	q = `SELECT
		m.match_date,
		b.player_id,
		m.format_id,
		COALESCE(m.venue_id, 0),
		COALESCE(mi.batting_team_opposition_id, 0),
		b.runs,
		b.balls,
		b.wickets,
		COALESCE(w.temp, 0), COALESCE(w.wind, 0), COALESCE(w.rain, 0), COALESCE(w.humidity, 0), COALESCE(w.cloud, 0), COALESCE(w.pressure, 0),
		CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
		COALESCE(mi.inning_number, 1),
		0 AS bowling_session,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		COALESCE(s.id, 0) AS season_id,
		p.player_name,
		COALESCE(fd.catches,0), COALESCE(fd.run_outs,0), COALESCE(fd.stumpings,0), COALESCE(fd.runouts_direct_hits,0),
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements,
		mf.code AS format_code
	FROM bowling_data b
	LEFT JOIN player p ON b.player_id = p.id
	LEFT JOIN (SELECT * FROM weather_data WHERE session = 'bowling') w ON b.match_id = w.match_id
	LEFT JOIN match_inning mi ON mi.match_id = b.match_id AND mi.inning_number = b.inning_number
	LEFT JOIN match m ON m.match_id = b.match_id
	LEFT JOIN match_format mf ON mf.id = m.format_id
	LEFT JOIN season s ON s.id = m.season_id
	LEFT JOIN fielding_data fd ON fd.match_id = b.match_id AND fd.player_id = b.player_id
	WHERE m.match_date < $1`
	args = []any{cutoff}
	if formatIDs != nil {
		q = strings.Replace(
			q,
			"WHERE m.match_date < $1",
			"WHERE m.format_id = ANY($1::bigint[]) AND m.match_date < $2",
			1,
		)
		args = []any{formatIDs, cutoff}
	}
	return q, args
}

type bowlingTrainingRowRaw struct {
	matchDate    time.Time
	playerID     int64
	formatID     int64
	venueID      int64
	oppositionID int64
	runs         string
	balls        string
	wickets      string
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
	formatCode   string
	catches      string
	runOuts      string
	stumpings    string
	runoutsDH    string
	fieldingInv  string
}

func bowlingTrainingRowsImpl(ctx context.Context, cutoff time.Time, formatIDs []int64) ([][]string, error) {
	q, args := bowlingTrainingRowsRawQuery(formatIDs, cutoff)
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rawRows []bowlingTrainingRowRaw
	for rows.Next() {
		var r bowlingTrainingRowRaw
		err := rows.Scan(
			&r.matchDate, &r.playerID, &r.formatID, &r.venueID, &r.oppositionID,
			&r.runs, &r.balls, &r.wickets,
			&r.temp, &r.wind, &r.rain, &r.humidity, &r.cloud, &r.pressure, &r.viscosity,
			&r.inning, &r.sess, &r.toss, &r.seasonID, &r.playerName,
			&r.catches, &r.runOuts, &r.stumpings, &r.runoutsDH, &r.fieldingInv,
			&r.formatCode,
		)
		if err != nil {
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Use UnixNano for map keys to avoid time.Time equality issues (monotonic clock).
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
	bulkRes, err := db.ListBowlingBeforeBulk(ctx, bulkKeys)
	if err != nil {
		return nil, fmt.Errorf("list bowling before bulk: %w", err)
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
		"runs", "balls", "wickets",
		"bowling_consistency", "bowling_form", "temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "bowling_session", "toss", "bowling_venue", "bowling_opposition", "season_id", "player_name",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements", "format_code",
	}
	alpha, lastN, windowN := GetFeatureExtractionParams()
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
		snap := computeBowlingSnapshotFromHistories(mainHist, venueHist, oppHist, r.matchDate, alpha, lastN, windowN)
		row := []string{
			r.runs, r.balls, r.wickets,
			floatToExport(snap.consistency), floatToExport(snap.form),
			r.temp, r.wind, r.rain, r.humidity, r.cloud, r.pressure, r.viscosity,
			r.inning, r.sess, r.toss,
			floatToExport(snap.venue), floatToExport(snap.opposition),
			r.seasonID, r.playerName,
			r.catches, r.runOuts, r.stumpings, r.runoutsDH, r.fieldingInv,
			r.formatCode,
		}
		out = append(out, row)
	}
	return out, nil
}

// BowlingTrainingRowsWithFormat returns bowling export-shaped rows for matches with match_date < cutoff
// and format_id in the given format's bucket (T20/T20I share a bucket). Form/consistency/venue/opposition
// are computed on the fly using the same EWM and Consistency logic as precompute-features.
func BowlingTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, fmt.Errorf("resolve format_id(s) for %s: %w", format, err)
	}
	return bowlingTrainingRowsImpl(ctx, cutoff, formatIDs)
}

// bowlingHoldoutRawQuery returns SQL and args for raw bowling rows for the given match IDs (same columns as training).
func bowlingHoldoutRawQuery(matchIDs []int64) (string, []any) {
	q := `SELECT
		m.match_date,
		b.player_id,
		m.format_id,
		COALESCE(m.venue_id, 0),
		COALESCE(mi.batting_team_opposition_id, 0),
		b.runs,
		b.balls,
		b.wickets,
		COALESCE(w.temp, 0), COALESCE(w.wind, 0), COALESCE(w.rain, 0), COALESCE(w.humidity, 0), COALESCE(w.cloud, 0), COALESCE(w.pressure, 0),
		CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
		COALESCE(mi.inning_number, 1),
		0 AS bowling_session,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		COALESCE(s.id, 0) AS season_id,
		p.player_name,
		COALESCE(fd.catches,0), COALESCE(fd.run_outs,0), COALESCE(fd.stumpings,0), COALESCE(fd.runouts_direct_hits,0),
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements,
		mf.code AS format_code
	FROM bowling_data b
	LEFT JOIN player p ON b.player_id = p.id
	LEFT JOIN (SELECT * FROM weather_data WHERE session = 'bowling') w ON b.match_id = w.match_id
	LEFT JOIN match_inning mi ON mi.match_id = b.match_id AND mi.inning_number = b.inning_number
	LEFT JOIN match m ON m.match_id = b.match_id
	LEFT JOIN match_format mf ON mf.id = m.format_id
	LEFT JOIN season s ON s.id = m.season_id
	LEFT JOIN fielding_data fd ON fd.match_id = b.match_id AND fd.player_id = b.player_id
	WHERE m.match_id = ANY($1::bigint[])`
	return q, []any{matchIDs}
}

// bowlingHoldoutRowsImpl returns bowling export-shaped rows for the given match IDs with features at cutoff.
func bowlingHoldoutRowsImpl(ctx context.Context, _ []int64, matchIDs []int64, cutoff time.Time) ([][]string, error) {
	if len(matchIDs) == 0 {
		headers := []string{
			"runs", "balls", "wickets",
			"bowling_consistency", "bowling_form", "temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
			"inning", "bowling_session", "toss", "bowling_venue", "bowling_opposition", "season_id", "player_name",
			"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements", "format_code",
		}
		return [][]string{headers}, nil
	}
	q, args := bowlingHoldoutRawQuery(matchIDs)
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rawRows []bowlingTrainingRowRaw
	for rows.Next() {
		var r bowlingTrainingRowRaw
		if err := rows.Scan(
			&r.matchDate, &r.playerID, &r.formatID, &r.venueID, &r.oppositionID,
			&r.runs, &r.balls, &r.wickets,
			&r.temp, &r.wind, &r.rain, &r.humidity, &r.cloud, &r.pressure, &r.viscosity,
			&r.inning, &r.sess, &r.toss, &r.seasonID, &r.playerName,
			&r.catches, &r.runOuts, &r.stumpings, &r.runoutsDH, &r.fieldingInv,
			&r.formatCode,
		); err != nil {
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
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
	bulkRes, err := db.ListBowlingBeforeBulk(ctx, bulkKeys)
	if err != nil {
		return nil, fmt.Errorf("list bowling before bulk: %w", err)
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
	alpha, lastN, windowN := GetFeatureExtractionParams()
	headers := []string{
		"runs", "balls", "wickets",
		"bowling_consistency", "bowling_form", "temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "bowling_session", "toss", "bowling_venue", "bowling_opposition", "season_id", "player_name",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements", "format_code",
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
		snap := computeBowlingSnapshotFromHistories(mainHist, venueHist, oppHist, cutoff, alpha, lastN, windowN)
		row := []string{
			r.runs, r.balls, r.wickets,
			floatToExport(snap.consistency), floatToExport(snap.form),
			r.temp, r.wind, r.rain, r.humidity, r.cloud, r.pressure, r.viscosity,
			r.inning, r.sess, r.toss,
			floatToExport(snap.venue), floatToExport(snap.opposition),
			r.seasonID, r.playerName,
			r.catches, r.runOuts, r.stumpings, r.runoutsDH, r.fieldingInv,
			r.formatCode,
		}
		out = append(out, row)
	}
	return out, nil
}

// BowlingHoldoutRows returns bowling export-shaped rows for the next limit matches after cutoff (walk-forward holdout).
func BowlingHoldoutRows(ctx context.Context, format string, cutoff time.Time, limit int) ([][]string, error) {
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
	return bowlingHoldoutRowsImpl(ctx, formatIDs, matchIDs, cutoff)
}
