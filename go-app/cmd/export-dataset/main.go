package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func main() {
	var outDir string
	flag.StringVar(&outDir, "out", "src/final_data/output", "output directory for exported CSVs")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}

	if err := exportBatting(ctx, filepath.Join(outDir, "batting_encoded.csv")); err != nil {
		log.Fatalf("export batting: %v", err)
	}
	if err := exportBowling(ctx, filepath.Join(outDir, "bowling_encoded.csv")); err != nil {
		log.Fatalf("export bowling: %v", err)
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
		p.batting_consistency,
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
		LEFT JOIN player_form_data pfd ON bd.player_id = pfd.player_id AND md.season_id = pfd.season_id`

	rows, err := db.Pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	// header
	w.Write(
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
	)
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
		p.bowling_consistency,
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
		LEFT JOIN player_form_data pfd ON b.player_id = pfd.player_id AND md.season_id = pfd.season_id`

	rows, err := db.Pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	// header
	w.Write(
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
	)
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
