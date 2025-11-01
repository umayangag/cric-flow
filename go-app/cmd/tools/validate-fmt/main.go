package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// validate-fmt: sanity-check coverage of per-format feature tables.
// It prints counts by (format_code, season_id) from match_details and from *_fmt tables,
// and exits non-zero when a format has matches but zero rows in the corresponding *_fmt tables.
func main() {
	var formatsCSV string
	flag.StringVar(
		&formatsCSV,
		"formats",
		"",
		"comma-separated list of format codes to validate (e.g., ODI,T20I). Empty = all",
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	// Load formats
	formatCodes := []string{}
	if strings.TrimSpace(formatsCSV) != "" {
		for _, f := range strings.Split(formatsCSV, ",") {
			f = strings.ToUpper(strings.TrimSpace(f))
			if f != "" {
				formatCodes = append(formatCodes, f)
			}
		}
	}
	if len(formatCodes) == 0 {
		rows, err := db.Pool.Query(ctx, `SELECT code FROM match_format ORDER BY code`)
		if err != nil {
			log.Fatalf("load formats: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				log.Fatalf("scan format: %v", err)
			}
			formatCodes = append(formatCodes, c)
		}
	}

	type key struct {
		format string
		season int64
	}
	matchCounts := map[key]int64{}
	formCounts := map[key]int64{}
	venueCounts := map[key]int64{}
	oppoCounts := map[key]int64{}

	// Gather counts from match_details (by season) for each format
	for _, code := range formatCodes {
		var formatID int64
		if err := db.Pool.QueryRow(ctx, `SELECT id FROM match_format WHERE code = $1`, code).Scan(&formatID); err != nil {
			log.Printf("warn: format %s not found in match_format; skipping", code)
			continue
		}
		rows, err := db.Pool.Query(
			ctx,
			`SELECT COALESCE(season_id, 0) AS season_id, COUNT(*) FROM match_details WHERE format_id=$1 GROUP BY season_id`,
			formatID,
		)
		if err != nil {
			log.Fatalf("query match_details: %v", err)
		}
		for rows.Next() {
			var sid int64
			var cnt int64
			if err := rows.Scan(&sid, &cnt); err != nil {
				log.Fatalf("scan md: %v", err)
			}
			matchCounts[key{code, sid}] = cnt
		}
		rows.Close()
		// Feature table counts (distinct keys)
		rows2, err := db.Pool.Query(
			ctx,
			`SELECT season_id, COUNT(*) FROM player_form_data_fmt WHERE format_id=$1 GROUP BY season_id`,
			formatID,
		)
		if err == nil {
			for rows2.Next() {
				var sid int64
				var cnt int64
				_ = rows2.Scan(&sid, &cnt)
				formCounts[key{code, sid}] = cnt
			}
			rows2.Close()
		}
		rows3, err := db.Pool.Query(
			ctx,
			`SELECT 0 as season_id, COUNT(*) FROM player_venue_data_fmt WHERE format_id=$1`,
			formatID,
		)
		if err == nil {
			for rows3.Next() {
				var sid int64
				var cnt int64
				_ = rows3.Scan(&sid, &cnt)
				venueCounts[key{code, sid}] = cnt
			}
			rows3.Close()
		}
		rows4, err := db.Pool.Query(
			ctx,
			`SELECT 0 as season_id, COUNT(*) FROM player_opposition_data_fmt WHERE format_id=$1`,
			formatID,
		)
		if err == nil {
			for rows4.Next() {
				var sid int64
				var cnt int64
				_ = rows4.Scan(&sid, &cnt)
				oppoCounts[key{code, sid}] = cnt
			}
			rows4.Close()
		}
	}

	// Print summary
	fmt.Println("format,season_id,matches,form_rows,venue_rows,opposition_rows")
	// Collect all keys
	all := map[key]struct{}{}
	for k := range matchCounts {
		all[k] = struct{}{}
	}
	for k := range formCounts {
		all[k] = struct{}{}
	}
	// Include venue/oppo with season 0
	for k := range venueCounts {
		all[k] = struct{}{}
	}
	for k := range oppoCounts {
		all[k] = struct{}{}
	}
	keys := make([]key, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].format == keys[j].format {
			return keys[i].season < keys[j].season
		}
		return keys[i].format < keys[j].format
	})
	bad := 0
	for _, k := range keys {
		mc := matchCounts[k]
		fc := formCounts[k]
		vc := venueCounts[key{k.format, 0}]
		oc := oppoCounts[key{k.format, 0}]
		fmt.Printf("%s,%d,%d,%d,%d,%d\n", k.format, k.season, mc, fc, vc, oc)
		if mc > 0 && (fc == 0 || vc == 0 || oc == 0) {
			bad++
		}
	}
	if bad > 0 {
		fmt.Fprintf(
			os.Stderr,
			"validation failed: %d gaps found (formats with matches but empty feature tables)\n",
			bad,
		)
		os.Exit(1)
	}
	fmt.Println("ok: per-format feature tables present for all formats with matches")
}
