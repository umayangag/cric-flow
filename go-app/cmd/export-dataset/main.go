// Command export-dataset exports training CSV datasets from the database.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	exportrepo "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/db/exportrepo"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/fsx/osfs"
	exportcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	expcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/exportdataset"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	exportsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/exportdataset"
)

const fieldingColumnsSQL = `
  COALESCE(fd.catches,0) AS catches,
  COALESCE(fd.run_outs,0) AS run_outs,
  COALESCE(fd.stumpings,0) AS stumpings,
  COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits,
  (COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0)) AS fielding_involvements,
`
const fieldingJoinSQL = "LEFT JOIN fielding_data fd ON fd.match_id = %s.match_id AND fd.player_id = %s.player_id"

func fieldingJoin(alias string) string { return fmt.Sprintf(fieldingJoinSQL, alias, alias) }

var fieldingHeaders = []string{
	"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements",
}

func main() {
	// Phase 2: delegate flag parsing and output dir preparation to internal packages.
	fs := flag.NewFlagSet("export-dataset", flag.ContinueOnError)
	opts, err := exportcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		// Preserve legacy behavior: print error and exit similar to flag.Parse failure.
		slog.Error("flag parsing failed", slog.Any("err", err))
		os.Exit(2)
	}
	// Ensure output directory precedence: flag > env (handled by parser) > config.DefaultExportDir()
	if opts.OutDir == "" {
		opts.OutDir = config.DefaultExportDir()
	}

	// Map options to legacy variables used later in this file while we migrate logic incrementally.
	var outDir string
	outDir = opts.OutDir

	logger.SetupFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Prepare filesystem via internal runner (creates outDir). Remove direct os.MkdirAll.

	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}

	// Wire internal services and runner to handle unified, legacy combined, and inference-only flows.
	fsys := osfs.New()
	repo := exportrepo.New()
	bat := exportsvc.NewBattingService(repo)
	bow := exportsvc.NewBowlingService(repo)
	runner := expcmd.NewRunnerWithServices(fsys, bat, bow)
	if runErr := runner.Run(ctx, opts); runErr != nil {
		slog.Error("runner execution failed", slog.Any("err", runErr))
		os.Exit(1)
	}
	// All flows are handled by Runner; log and return.
	slog.Info("exports written", slog.String("dir", outDir))
	return
}

func exportBatting(ctx context.Context, path string) error {
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
		md.inning,
		CASE 
			WHEN md.batting_session IS NULL THEN 0
			WHEN lower(md.batting_session) LIKE '%morning%' THEN 0
			WHEN lower(md.batting_session) LIKE '%afternoon%' THEN 1
			WHEN lower(md.batting_session) LIKE '%evening%' THEN 2
			ELSE 0
		END AS batting_session,
		CASE 
			WHEN md.toss IS NULL THEN 0
			WHEN lower(md.toss) LIKE '%bat%' THEN 1
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
		LEFT JOIN match_details md ON md.match_id = w.match_id
		LEFT JOIN venue v ON v.id = md.venue_id
		LEFT JOIN opposition o ON o.id = md.opposition_id
		LEFT JOIN season s ON s.id = md.season_id
		-- Latest overall batting form as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_form
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		-- Latest overall batting consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_consistency
		  FROM feature_consistency_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest batting form vs opposition as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_opposition
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		-- Latest batting form at venue as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_venue
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE`

	rows, err := db.Pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	defer w.Flush()
	// header
	if err := w.Write(
		[]string{
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
		},
	); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, 21)
		if err != nil {
			return err
		}
		if err := w.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

func exportBowling(ctx context.Context, path string) error {
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
		return err
	}
	defer rows.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	defer w.Flush()
	// header
	if err := w.Write(
		[]string{
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
		},
	); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, 19)
		if err != nil {
			return err
		}
		if err := w.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// scanRow reads count columns from pgx.Rows into string representations suitable for CSV.
func scanRow(rows pgx.Rows, count int) ([]string, error) {
	scan := make([]any, count)
	buf := make([]string, count)
	for i := 0; i < count; i++ {
		var v any
		scan[i] = &v
	}
	if err := rows.Scan(scan...); err != nil {
		return nil, err
	}
	for i := 0; i < count; i++ {
		v := *(scan[i].(*any))
		if v == nil {
			buf[i] = ""
			continue
		}
		switch t := v.(type) {
		case []byte:
			buf[i] = string(t)
		default:
			buf[i] = toString(t)
		}
	}
	return buf, nil
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int64:
		return intToString(int(t))
	case int32:
		return intToString(int(t))
	case int:
		return intToString(t)
	case float32:
		return floatToString(float64(t))
	case float64:
		return floatToString(t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return ""
	}
}

func intToString(i int) string       { return fmt.Sprintf("%d", i) }
func floatToString(f float64) string { return fmt.Sprintf("%g", f) }

// exportBattingLegacy is the previous exporter (no format filter).
func exportBattingLegacy(ctx context.Context, path string) error { return exportBatting(ctx, path) }

// exportBowlingLegacy is the previous exporter (no format filter).
func exportBowlingLegacy(ctx context.Context, path string) error { return exportBowling(ctx, path) }

// exportBattingUnified writes a single batting CSV across all formats and includes per-format as-of features
func exportBattingUnified(ctx context.Context, path string) error {
	// Resolve known format ids (some may be missing depending on data)
	fmtIDs := map[string]*int64{}
	for _, code := range []string{"TEST", "ODI", "T20I", "T20"} {
		id, err := db.GetMatchFormatIDByCode(ctx, code)
		if err == nil && id > 0 {
			fmtIDs[code] = &id
		}
	}
	// Build SQL selecting base batting row + as-of features per available format code
	// We use LATERAL subqueries to fetch latest snapshot <= match date.
	q := `
	SELECT 
	  bd.runs, bd.balls, bd.fours, bd.sixes, bd.batting_position,
	  w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure,
	  CASE WHEN w.viscosity IS NULL THEN 0
	       WHEN lower(w.viscosity)='dry' THEN 0
	       WHEN lower(w.viscosity)='humid' THEN 1
	       WHEN lower(w.viscosity)='windy' THEN 2
	       ELSE 0 END AS viscosity,
	  md.inning,
	  CASE WHEN md.batting_session IS NULL THEN 0
	       WHEN lower(md.batting_session) LIKE '%morning%' THEN 0
	       WHEN lower(md.batting_session) LIKE '%afternoon%' THEN 1
	       WHEN lower(md.batting_session) LIKE '%evening%' THEN 2 ELSE 0 END AS batting_session,
	  CASE WHEN md.toss IS NULL THEN 0 WHEN lower(md.toss) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
	  s.id AS season_id,
	  p.player_name,
	  mf.code AS format_code,
	  ` + fieldingColumnsSQL + `
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
	JOIN match_details md ON md.match_id = bd.match_id
	LEFT JOIN match_format mf ON mf.id = md.format_id
	LEFT JOIN player p ON p.id = bd.player_id
	LEFT JOIN season s ON s.id = md.season_id
	LEFT JOIN (
	  SELECT * FROM weather_data WHERE session='batting'
	) w ON w.match_id = bd.match_id
	` + fieldingJoin("bd") + `
	-- TEST lateral joins
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $1 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) tf ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
	  WHERE player_id=bd.player_id AND format_id = $1 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) tc ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $1 AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) tvo ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $1 AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) tvv ON TRUE
	-- ODI lateral joins
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $2 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) of ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
	  WHERE player_id=bd.player_id AND format_id = $2 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) oc ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $2 AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) ovo ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $2 AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) ovv ON TRUE
	-- T20I lateral joins
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $3 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) iif ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
	  WHERE player_id=bd.player_id AND format_id = $3 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) iic ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $3 AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) iivo ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $3 AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) iivv ON TRUE
	-- T20 lateral joins
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $4 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) t20f ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
	  WHERE player_id=bd.player_id AND format_id = $4 AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) t20c ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $4 AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) t20vo ON TRUE
	LEFT JOIN LATERAL (
	  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
	  WHERE player_id=bd.player_id AND format_id = $4 AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date ORDER BY as_of_date DESC LIMIT 1
	) t20vv ON TRUE
	ORDER BY md.date ASC, bd.id ASC`

	args := []any{nil, nil, nil, nil}
	// Parameter order: TEST, ODI, T20I, T20
	idx := 0
	set := func(code string) {
		if v, ok := fmtIDs[code]; ok && v != nil {
			args[idx] = *v
		} else {
			args[idx] = int64(0) // no matches will appear for format_id=0 (since none equals 0)
		}
		idx++
	}
	set("TEST")
	set("ODI")
	set("T20I")
	set("T20")

	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"runs", "balls", "fours", "sixes", "batting_position",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "batting_session", "toss", "season_id", "player_name", "format_code",
	}
	// Fielding columns appear immediately after format_code in the SELECT list; keep header aligned.
	header = append(header, fieldingHeaders...)
	header = append(header,
		"bat_form_TEST_asof", "bat_consistency_TEST_asof", "bat_vs_opp_TEST_asof", "bat_at_venue_TEST_asof",
		"bat_form_ODI_asof", "bat_consistency_ODI_asof", "bat_vs_opp_ODI_asof", "bat_at_venue_ODI_asof",
		"bat_form_T20I_asof", "bat_consistency_T20I_asof", "bat_vs_opp_T20I_asof", "bat_at_venue_T20I_asof",
		"bat_form_T20_asof", "bat_consistency_T20_asof", "bat_vs_opp_T20_asof", "bat_at_venue_T20_asof",
	)
	if err := w.Write(header); err != nil {
		return err
	}

	// total columns expected from query
	expected := len(header)
	for rows.Next() {
		vals, err := scanRow(rows, expected)
		if err != nil {
			return err
		}
		if err := w.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// exportBowlingUnified writes a single bowling CSV across all formats and includes per-format as-of features
func exportBowlingUnified(ctx context.Context, path string) error {
	fmtIDs := map[string]*int64{}
	for _, code := range []string{"TEST", "ODI", "T20I", "T20"} {
		id, err := db.GetMatchFormatIDByCode(ctx, code)
		if err == nil && id > 0 {
			fmtIDs[code] = &id
		}
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
	 ` + fieldingColumnsSQL + `
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
	` + fieldingJoin("bw") + `
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
	) t20vv ON TRUE
	ORDER BY md.date ASC, bw.id ASC`

	args := []any{nil, nil, nil, nil}
	idx := 0
	for _, code := range []string{"TEST", "ODI", "T20I", "T20"} {
		if v, ok := fmtIDs[code]; ok && v != nil {
			args[idx] = *v
		} else {
			args[idx] = int64(0)
		}
		idx++
	}
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"overs", "balls", "maidens", "runs", "wickets", "dots", "fours", "sixes", "econ", "wides", "no_balls",
		"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity",
		"inning", "bowling_session", "toss", "season_id", "player_name", "format_code",
	}
	// Fielding columns are selected immediately after format_code in the query
	header = append(header, fieldingHeaders...)
	// Per-format as-of feature columns follow fielding columns in the SELECT order
	header = append(header,
		"bowl_form_TEST_asof", "bowl_consistency_TEST_asof", "bowl_vs_opp_TEST_asof", "bowl_at_venue_TEST_asof",
		"bowl_form_ODI_asof", "bowl_consistency_ODI_asof", "bowl_vs_opp_ODI_asof", "bowl_at_venue_ODI_asof",
		"bowl_form_T20I_asof", "bowl_consistency_T20I_asof", "bowl_vs_opp_T20I_asof", "bowl_at_venue_T20I_asof",
		"bowl_form_T20_asof", "bowl_consistency_T20_asof", "bowl_vs_opp_T20_asof", "bowl_at_venue_T20_asof",
	)
	if err := w.Write(header); err != nil {
		return err
	}

	expected := len(header)
	for rows.Next() {
		vals, err := scanRow(rows, expected)
		if err != nil {
			return err
		}
		if err := w.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// exportBattingFormat writes a batting CSV filtered by a specific match format code using *_fmt tables.
func exportBattingFormat(ctx context.Context, formatCode string, path string) error {
	// Resolve format_id for the provided code
	formatID, err := db.GetMatchFormatIDByCode(ctx, strings.ToUpper(strings.TrimSpace(formatCode)))
	if err != nil {
		return fmt.Errorf("resolve format_id for %s: %w", formatCode, err)
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
		md.inning,
		CASE 
			WHEN md.batting_session IS NULL THEN 0
			WHEN lower(md.batting_session) LIKE '%morning%' THEN 0
			WHEN lower(md.batting_session) LIKE '%afternoon%' THEN 1
			WHEN lower(md.batting_session) LIKE '%evening%' THEN 2
			ELSE 0
		END AS batting_session,
		CASE 
			WHEN md.toss IS NULL THEN 0
			WHEN lower(md.toss) LIKE '%bat%' THEN 1
			ELSE 0
		END AS toss,
		tvv.batting_venue,
		tvo.batting_opposition,
		s.id AS season_id,
		p.player_name,
		` + fieldingColumnsSQL + `
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'batting'
		) w ON bd.match_id = w.match_id
		LEFT JOIN match_details md ON md.match_id = bd.match_id
		LEFT JOIN season s ON s.id = md.season_id
		-- Latest overall batting form as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_form, n_samples_bat FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		-- Latest overall batting consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_consistency, n_samples_bat FROM feature_consistency_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest batting form vs opposition as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_opposition, n_samples_bat AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		-- Latest batting form at venue as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_venue, n_samples_bat AS n_samples FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE ` + fieldingJoin("bd") + ` WHERE md.format_id = $1`

	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	defer w.Flush()
	// header
	if err := w.Write(
		[]string{
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
			"format_code",
		},
	); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, len(rows.FieldDescriptions()))
		if err != nil {
			return err
		}
		// append format_code as last column
		vals = append(vals, strings.ToUpper(strings.TrimSpace(formatCode)))
		if err := w.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// exportBowlingFormat writes a bowling CSV filtered by a specific match format code using *_fmt tables.
func exportBowlingFormat(ctx context.Context, formatCode string, path string) error {
	formatID, err := db.GetMatchFormatIDByCode(ctx, strings.ToUpper(strings.TrimSpace(formatCode)))
	if err != nil {
		return fmt.Errorf("resolve format_id for %s: %w", formatCode, err)
	}
	q := `SELECT  
		b.runs,
		b.balls,
		b.wickets,
		tc.bowling_consistency,
		tf.bowling_form,
		w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure,
		CASE 
			WHEN w.viscosity IS NULL THEN 0
			WHEN lower(w.viscosity) = 'dry' THEN 0
			WHEN lower(w.viscosity) = 'humid' THEN 1
			WHEN lower(w.viscosity) = 'windy' THEN 2
			ELSE 0
		END AS viscosity,
		md.inning,
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
		tvv.bowling_venue,
		tvo.bowling_opposition,
		s.id AS season_id,
		p.player_name,
		` + fieldingColumnsSQL + `
		FROM bowling_data b
		LEFT JOIN player p ON b.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'bowling'
		) w ON b.match_id = w.match_id
		LEFT JOIN match_details md ON md.match_id = b.match_id
		LEFT JOIN season s ON s.id = md.season_id
		-- Latest overall bowling form as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_form, n_samples_bowl FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		-- Latest overall bowling consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_consistency, n_samples_bowl FROM feature_consistency_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest bowling form vs opposition as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_opposition, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		-- Latest bowling form at venue as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT bowling_value AS bowling_venue, n_samples_bowl AS n_samples FROM feature_form_snapshots
		  WHERE player_id=b.player_id AND format_id = md.format_id AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE ` + fieldingJoin("b") + ` WHERE md.format_id = $1`

	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	defer w.Flush()
	// header
	if err := w.Write(
		[]string{
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
		},
	); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, len(rows.FieldDescriptions()))
		if err != nil {
			return err
		}
		vals = append(vals, strings.ToUpper(strings.TrimSpace(formatCode)))
		if err := w.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// exportBattingFormatInference writes inputs-only batting CSV in inference order for a specific match format code.
func exportBattingFormatInference(ctx context.Context, formatCode string, path string) error {
	formatID, err := db.GetMatchFormatIDByCode(ctx, strings.ToUpper(strings.TrimSpace(formatCode)))
	if err != nil {
		return fmt.Errorf("resolve format_id for %s: %w", formatCode, err)
	}
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
		COALESCE(md.inning, 1) AS batting_inning,
		CASE 
			WHEN md.batting_session IS NULL THEN 0
			WHEN lower(md.batting_session) LIKE '%morning%' THEN 0
			WHEN lower(md.batting_session) LIKE '%afternoon%' THEN 1
			WHEN lower(md.batting_session) LIKE '%evening%' THEN 2
			ELSE 0
		END AS batting_session,
		CASE 
			WHEN md.toss IS NULL THEN 0
			WHEN lower(md.toss) LIKE '%bat%' THEN 1
			ELSE 0
		END AS toss,
		COALESCE(tvv.batting_venue, 0) AS venue,
		COALESCE(tvo.batting_opposition, 0) AS opposition,
		COALESCE(s.id, 0) AS season,
		p.player_name,
		` + fieldingColumnsSQL + `
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'batting'
		) w ON bd.match_id = w.match_id
		LEFT JOIN match_details md ON md.match_id = bd.match_id
		LEFT JOIN season s ON s.id = md.season_id
		-- Latest overall batting form and consistency as-of match date (per-format)
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_form
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tf ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_consistency
		  FROM feature_consistency_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tc ON TRUE
		-- Latest batting form vs opposition and at venue
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_opposition
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='opposition' AND scope_id = md.opposition_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvo ON TRUE
		LEFT JOIN LATERAL (
		  SELECT batting_value AS batting_venue
		  FROM feature_form_snapshots
		  WHERE player_id=bd.player_id AND format_id = md.format_id AND scope='venue' AND scope_id = md.venue_id AND as_of_date <= md.date
		  ORDER BY as_of_date DESC LIMIT 1
		) tvv ON TRUE ` + fieldingJoin("bd") + ` WHERE md.format_id = $1`

	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	wrt := csv.NewWriter(f)
	defer wrt.Flush()
	// header in exact inference order
	if err := wrt.Write([]string{
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
	}); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, 21)
		if err != nil {
			return err
		}
		if err := wrt.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// exportBowlingFormatInference writes inputs-only bowling CSV in inference order for a specific match format code.
func exportBowlingFormatInference(ctx context.Context, formatCode string, path string) error {
	formatID, err := db.GetMatchFormatIDByCode(ctx, strings.ToUpper(strings.TrimSpace(formatCode)))
	if err != nil {
		return fmt.Errorf("resolve format_id for %s: %w", formatCode, err)
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
		` + fieldingColumnsSQL + `
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
		) tvv ON TRUE ` + fieldingJoin("b") + ` WHERE md.format_id = $1`

	rows, err := db.Pool.Query(ctx, q, formatID)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	wrt := csv.NewWriter(f)
	defer wrt.Flush()
	if err := wrt.Write([]string{
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
	}); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, 21)
		if err != nil {
			return err
		}
		if err := wrt.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}
