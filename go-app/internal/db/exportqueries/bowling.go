package exportqueries

import (
	"context"
	"fmt"
	"strings"

	pgx "github.com/jackc/pgx/v5"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// BowlingUnifiedRows returns CSV-shaped rows for the unified bowling export.
// Mirrors legacy exportBowlingUnified: base bowling row + per-format as-of features and fielding.
func BowlingUnifiedRows(ctx context.Context) ([][]string, error) {
	// Resolve known format ids similar to legacy
	ids := make([]any, 0, 4)
	codes := []string{"TEST", "ODI", "T20I", "T20"}
	for _, code := range codes {
		id, err := db.GetMatchFormatIDByCode(ctx, code)
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
	  md.inning,
	  CASE WHEN md.bowling_session IS NULL THEN 0
	       WHEN lower(md.bowling_session) LIKE '%morning%' THEN 0
	       WHEN lower(md.bowling_session) LIKE '%afternoon%' THEN 1
	       WHEN lower(md.bowling_session) LIKE '%evening%' THEN 2 ELSE 0 END AS bowling_session,
	  CASE WHEN md.toss IS NULL THEN 0 WHEN lower(md.toss) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
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
		JOIN match_details md ON md.match_id = bw.match_id
		LEFT JOIN match_format mf ON mf.id = md.format_id
		LEFT JOIN player p ON p.id = bw.player_id
		LEFT JOIN season s ON s.id = md.season_id
		LEFT JOIN (
		  SELECT * FROM weather_data WHERE session='bowling'
		) w ON w.match_id = bw.match_id
		LEFT JOIN fielding_data fd ON fd.match_id = bw.match_id AND fd.player_id = bw.player_id
		-- TEST laterals
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $1 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=bw.player_id AND format_id = $1 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $1 AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $1 AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE
		-- ODI laterals
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $2 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) of ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=bw.player_id AND format_id = $2 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) oc ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $2 AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) ovo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $2 AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) ovv ON TRUE
		-- T20I laterals
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $3 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) iif ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=bw.player_id AND format_id = $3 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) iic ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $3 AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) iivo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $3 AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) iivv ON TRUE
		-- T20 laterals
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $4 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) t20f ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=bw.player_id AND format_id = $4 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) t20c ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $4 AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) t20vo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowl_value, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bw.player_id AND format_id = $4 AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
		) t20vv ON TRUE`
	rows, err := db.Pool.Query(ctx, q, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{
		"overs", "balls", "maidens", "runs", "wickets", "dots", "fours", "sixes", "econ", "wides", "no_balls",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "bowling_session", "toss", "season_id", "player_name", "format_code",
		"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
		"bowl_form_TEST_asof", "bowl_consistency_TEST_asof", "bowl_vs_opp_TEST_asof", "bowl_at_venue_TEST_asof",
		"bowl_form_ODI_asof", "bowl_consistency_ODI_asof", "bowl_vs_opp_ODI_asof", "bowl_at_venue_ODI_asof",
		"bowl_form_T20I_asof", "bowl_consistency_T20I_asof", "bowl_vs_opp_T20I_asof", "bowl_at_venue_T20I_asof",
		"bowl_form_T20_asof", "bowl_consistency_T20_asof", "bowl_vs_opp_T20_asof", "bowl_at_venue_T20_asof",
	}
	out := make([][]string, 0, 2048)
	out = append(out, headers)
	for rows.Next() {
		vals, err := scanToStringsB(rows, len(headers))
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
		md.inning,
		md.bowling_session,
		md.toss,
		tvv.bowling_venue,
		tvo.bowling_opposition,
		s.id AS season_id,
		p.player_name
		FROM bowling_data b
		LEFT JOIN player p ON b.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'bowling'
		) w ON b.match_id = w.match_id
		LEFT JOIN match_details md ON md.match_id = w.match_id
		LEFT JOIN venue v ON v.id = md.venue_id
		LEFT JOIN opposition o ON o.id = md.opposition_id
		LEFT JOIN season s ON s.id = md.season_id
		-- Latest overall bowling form as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_form
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		-- Latest overall bowling consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_consistency
		  FROM feature_consistency_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest bowling form vs opposition as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_opposition
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		-- Latest bowling form at venue as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_venue
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date
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
		vals, err := scanToStringsB(rows, len(headers))
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
	formatID, err := db.GetMatchFormatIDByCode(ctx, strings.ToUpper(strings.TrimSpace(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format_id for %s: %w", format, err)
	}
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
		COALESCE(md.inning, 1) AS batting_inning,
		CASE 
			WHEN md.bowling_session IS NULL THEN 0
			WHEN lower(md.bowling_session) LIKE '%morning%' THEN 0
			WHEN lower(md.bowling_session) LIKE '%afternoon%' THEN 1
			WHEN lower(md.bowling_session) LIKE '%evening%' THEN 2
			ELSE 0
		END AS bowling_session,
		CASE 
			WHEN md.toss IS NULL THEN 0
			WHEN lower(md.toss) LIKE '%bat%' THEN 1
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
		LEFT JOIN match_details md ON md.match_id = b.match_id
		LEFT JOIN season s ON s.id = md.season_id
		-- Latest overall bowling form and consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_form
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_consistency
		  FROM feature_consistency_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest bowling form vs opposition and at venue
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_opposition
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_venue
		  FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE LEFT JOIN fielding_data fd ON fd.match_id = b.match_id AND fd.player_id = b.player_id WHERE md.format_id = $1`
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
		vals, err := scanToStringsB(rows, len(headers))
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
	formatID, err := db.GetMatchFormatIDByCode(ctx, strings.ToUpper(strings.TrimSpace(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format_id for %s: %w", format, err)
	}
	q := `SELECT  
		b.runs,
		b.balls,
		b.wickets,
		tc.bowling_consistency,
		tf.bowling_form,
		w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure, w.viscosity,
		md.inning,
		md.bowling_session,
		md.toss,
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
		LEFT JOIN match_details md ON md.match_id = b.match_id
		LEFT JOIN season s ON s.id = md.season_id
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_opposition, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_venue, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE WHERE md.format_id = $1`
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
		vals, err := scanToStringsB(rows, len(headers)-1)
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

// scanToStringsB mirrors scanToStrings with a local copy to avoid cross-file deps.
func scanToStringsB(r pgx.Rows, n int) ([]string, error) {
	dests := make([]any, n)
	for i := 0; i < n; i++ {
		var v any
		dests[i] = &v
	}
	if err := r.Scan(dests...); err != nil {
		return nil, err
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		v := *(dests[i].(*any))
		out[i] = anyToStringB(v)
	}
	return out, nil
}

func anyToStringB(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return fmt.Sprintf("%d", t)
	case int32:
		return fmt.Sprintf("%d", t)
	case int:
		return fmt.Sprintf("%d", t)
	case float32:
		return trimFloatB(fmt.Sprintf("%g", float64(t)))
	case float64:
		return trimFloatB(fmt.Sprintf("%g", t))
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", t)
	}
}

func trimFloatB(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}
