// Command export-dataset exports training CSV datasets from the database.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func main() {
	var outDir string
	var format string
	var formats string
	var allFormats bool
	var inferenceOnly bool
	// Resolve default output directory with precedence: flag > env > config > built-in
	defOut := os.Getenv("GO_APP_OUTPUT_DIR")
	if defOut == "" {
		defOut = config.DefaultExportDir()
	}
	flag.StringVar(
		&outDir,
		"out",
		defOut,
		"output directory for exported CSVs (default from env GO_APP_OUTPUT_DIR or config.json)",
	)
	flag.StringVar(&format, "format", "", "single format code (TEST, ODI, T20, T20I)")
	flag.StringVar(&formats, "formats", "", "comma-separated list of format codes")
	flag.BoolVar(&allFormats, "all-formats", false, "export for all formats")
	flag.BoolVar(&inferenceOnly, "inference-only", false, "emit inputs-only CSVs for inference (separate files)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}

	var list []string
	if allFormats {
		list = []string{"TEST", "ODI", "T20", "T20I"}
	} else if formats != "" {
		for _, c := range strings.Split(formats, ",") {
			c = strings.TrimSpace(strings.ToUpper(c))
			if c != "" {
				list = append(list, c)
			}
		}
	} else if format != "" {
		list = []string{strings.ToUpper(strings.TrimSpace(format))}
	}
	if len(list) == 0 {
		cfg := config.Load()
		if cfg.Export.RequiredFormat != "" {
			list = []string{strings.ToUpper(strings.TrimSpace(cfg.Export.RequiredFormat))}
		} else if cfg.Export.SplitByFormat {
			list = []string{"TEST", "ODI", "T20", "T20I"}
		} else {
			// Backward-compat: single unsuffixed files using all formats combined (legacy)
			list = []string{""}
		}
	}

	for _, fcode := range list {
		if fcode == "" {
			// Legacy one-shot (no filter, legacy joins)
			if inferenceOnly {
				log.Printf("skipping legacy inference-only exports; please specify --format/--formats/--all-formats")
				continue
			}
			if err := exportBattingLegacy(ctx, filepath.Join(outDir, "batting_encoded.csv")); err != nil {
				log.Fatalf("export batting (legacy): %v", err)
			}
			if err := exportBowlingLegacy(ctx, filepath.Join(outDir, "bowling_encoded.csv")); err != nil {
				log.Fatalf("export bowling (legacy): %v", err)
			}
			continue
		}
		if inferenceOnly {
			batInfer := filepath.Join(outDir, fmt.Sprintf("batting_infer_%s.csv", fcode))
			bowInfer := filepath.Join(outDir, fmt.Sprintf("bowling_infer_%s.csv", fcode))
			if err := exportBattingFormatInference(ctx, fcode, batInfer); err != nil {
				log.Fatalf("export batting inference(%s): %v", fcode, err)
			}
			if err := exportBowlingFormatInference(ctx, fcode, bowInfer); err != nil {
				log.Fatalf("export bowling inference(%s): %v", fcode, err)
			}
			continue
		}
		bat := filepath.Join(outDir, fmt.Sprintf("batting_encoded_%s.csv", fcode))
		bow := filepath.Join(outDir, fmt.Sprintf("bowling_encoded_%s.csv", fcode))
		if err := exportBattingFormat(ctx, fcode, bat); err != nil {
			log.Fatalf("export batting(%s): %v", fcode, err)
		}
		if err := exportBowlingFormat(ctx, fcode, bow); err != nil {
			log.Fatalf("export bowling(%s): %v", fcode, err)
		}
	}
	log.Printf("exports written to %s", outDir)
}

func exportBatting(ctx context.Context, path string) error {
	const q = `SELECT  
		bd.runs,
		bd.balls,
		bd.fours,
		bd.sixes,
		bd.batting_position,
		pcd.batting_consistency,
		pfd.batting_form,
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
		pvd.batting_venue,
		pod.batting_opposition,
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
		LEFT JOIN player_venue_data pvd ON bd.player_id = pvd.player_id AND md.venue_id = pvd.venue_id
		LEFT JOIN player_opposition_data pod ON bd.player_id = pod.player_id AND md.opposition_id = pod.opposition_id
		LEFT JOIN player_form_data pfd ON bd.player_id = pfd.player_id AND md.season_id = pfd.season_id
		LEFT JOIN player_consistency_data_fmt pcd ON bd.player_id = pcd.player_id AND md.season_id = pcd.season_id AND md.format_id = pcd.format_id`

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
		pcd.bowling_consistency,
		pfd.bowling_form,
		w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure, w.viscosity,
		md.inning,
		md.bowling_session,
		md.toss,
		pvd.bowling_venue,
		pod.bowling_opposition,
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
		LEFT JOIN player_venue_data pvd ON b.player_id = pvd.player_id AND md.venue_id = pvd.venue_id
		LEFT JOIN player_opposition_data pod ON b.player_id = pod.player_id AND md.opposition_id = pod.opposition_id
		LEFT JOIN player_form_data pfd ON b.player_id = pfd.player_id AND md.season_id = pfd.season_id
		LEFT JOIN player_consistency_data_fmt pcd ON b.player_id = pcd.player_id AND md.season_id = pcd.season_id AND md.format_id = pcd.format_id`

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

// exportBattingFormat writes a batting CSV filtered by a specific match format code using *_fmt tables.
func exportBattingFormat(ctx context.Context, formatCode string, path string) error {
	// Resolve format_id for the provided code
	formatID, err := db.GetMatchFormatIDByCode(ctx, strings.ToUpper(strings.TrimSpace(formatCode)))
	if err != nil {
		return fmt.Errorf("resolve format_id for %s: %w", formatCode, err)
	}
	const q = `SELECT  
		bd.runs,
		bd.balls,
		bd.fours,
		bd.sixes,
		bd.batting_position,
		pcd.batting_consistency,
		pfd.batting_form,
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
		pvd.batting_venue,
		pod.batting_opposition,
		s.id AS season_id,
		p.player_name
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'batting'
		) w ON bd.match_id = w.match_id
		LEFT JOIN match_details md ON md.match_id = bd.match_id
		LEFT JOIN season s ON s.id = md.season_id
		LEFT JOIN player_venue_data_fmt pvd ON bd.player_id = pvd.player_id AND md.venue_id = pvd.venue_id AND md.format_id = pvd.format_id
		LEFT JOIN player_opposition_data_fmt pod ON bd.player_id = pod.player_id AND md.opposition_id = pod.opposition_id AND md.format_id = pod.format_id
		LEFT JOIN player_form_data_fmt pfd ON bd.player_id = pfd.player_id AND md.season_id = pfd.season_id AND md.format_id = pfd.format_id
		LEFT JOIN player_consistency_data_fmt pcd ON bd.player_id = pcd.player_id AND md.season_id = pcd.season_id AND md.format_id = pcd.format_id
		WHERE md.format_id = $1`

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
			"format_code",
		},
	); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, 21)
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
	const q = `SELECT  
		b.runs,
		b.balls,
		b.wickets,
		pcd.bowling_consistency,
		pfd.bowling_form,
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
		pvd.bowling_venue,
		pod.bowling_opposition,
		s.id AS season_id,
		p.player_name
		FROM bowling_data b
		LEFT JOIN player p ON b.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'bowling'
		) w ON b.match_id = w.match_id
		LEFT JOIN match_details md ON md.match_id = b.match_id
		LEFT JOIN season s ON s.id = md.season_id
		LEFT JOIN player_venue_data_fmt pvd ON b.player_id = pvd.player_id AND md.venue_id = pvd.venue_id AND md.format_id = pvd.format_id
		LEFT JOIN player_opposition_data_fmt pod ON b.player_id = pod.player_id AND md.opposition_id = pod.opposition_id AND md.format_id = pod.format_id
		LEFT JOIN player_form_data_fmt pfd ON b.player_id = pfd.player_id AND md.season_id = pfd.season_id AND md.format_id = pfd.format_id
		LEFT JOIN player_consistency_data_fmt pcd ON b.player_id = pcd.player_id AND md.season_id = pcd.season_id AND md.format_id = pcd.format_id
		WHERE md.format_id = $1`

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
			"format_code",
		},
	); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, 19)
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
	const q = `SELECT  
		pcd.batting_consistency,
		pfd.batting_form,
		w.temp,
		w.wind,
		w.rain,
		w.humidity,
		w.cloud,
		w.pressure,
		CASE 
			WHEN w.viscosity IS NULL THEN 0
			WHEN lower(w.viscosity) = 'humid' THEN 1
			ELSE 0
		END AS batting_viscosity,
		md.inning AS batting_inning,
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
		pvd.batting_venue AS venue,
		pod.batting_opposition AS opposition,
		s.id AS season,
		p.player_name
		FROM batting_data bd
		LEFT JOIN player p ON bd.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'batting'
		) w ON bd.match_id = w.match_id
		LEFT JOIN match_details md ON md.match_id = bd.match_id
		LEFT JOIN season s ON s.id = md.season_id
		LEFT JOIN player_venue_data_fmt pvd ON bd.player_id = pvd.player_id AND md.venue_id = pvd.venue_id AND md.format_id = pvd.format_id
		LEFT JOIN player_opposition_data_fmt pod ON bd.player_id = pod.player_id AND md.opposition_id = pod.opposition_id AND md.format_id = pod.format_id
		LEFT JOIN player_form_data_fmt pfd ON bd.player_id = pfd.player_id AND md.season_id = pfd.season_id AND md.format_id = pfd.format_id
		LEFT JOIN player_consistency_data_fmt pcd ON bd.player_id = pcd.player_id AND md.season_id = pcd.season_id AND md.format_id = pcd.format_id
		WHERE md.format_id = $1`

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
	}); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, 16)
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
	const q = `SELECT  
		pcd.bowling_consistency,
		pfd.bowling_form,
		w.temp,
		w.wind,
		w.rain,
		w.humidity,
		w.cloud,
		w.pressure,
		CASE 
			WHEN w.viscosity IS NULL THEN 0
			WHEN lower(w.viscosity) = 'dry' THEN 0
			WHEN lower(w.viscosity) = 'humid' THEN 1
			WHEN lower(w.viscosity) = 'windy' THEN 2
			ELSE 0
		END AS bowling_viscosity,
		md.inning AS batting_inning,
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
		pvd.bowling_venue,
		pod.bowling_opposition,
		s.id AS season,
		p.player_name
		FROM bowling_data b
		LEFT JOIN player p ON b.player_id = p.id
		LEFT JOIN (
			SELECT * FROM weather_data WHERE session = 'bowling'
		) w ON b.match_id = w.match_id
		LEFT JOIN match_details md ON md.match_id = b.match_id
		LEFT JOIN season s ON s.id = md.season_id
		LEFT JOIN player_venue_data_fmt pvd ON b.player_id = pvd.player_id AND md.venue_id = pvd.venue_id AND md.format_id = pvd.format_id
		LEFT JOIN player_opposition_data_fmt pod ON b.player_id = pod.player_id AND md.opposition_id = pod.opposition_id AND md.format_id = pod.format_id
		LEFT JOIN player_form_data_fmt pfd ON b.player_id = pfd.player_id AND md.season_id = pfd.season_id AND md.format_id = pfd.format_id
		LEFT JOIN player_consistency_data_fmt pcd ON b.player_id = pcd.player_id AND md.season_id = pcd.season_id AND md.format_id = pcd.format_id
		WHERE md.format_id = $1`

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
	}); err != nil {
		return err
	}
	for rows.Next() {
		vals, err := scanRow(rows, 16)
		if err != nil {
			return err
		}
		if err := wrt.Write(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}
