// Command precompute-features replays matches chronologically and persists
// leakage-free, date-indexed (as-of) feature snapshots per player.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
)

func main() {
	var (
		formatCode = flag.String("format", "ODI", "Match format code: TEST|ODI|T20|T20I")
		fromStr    = flag.String("from", "", "Start date (YYYY-MM-DD), optional")
		toStr      = flag.String("to", "", "End date (YYYY-MM-DD), optional")
		alpha      = flag.Float64("ewm-alpha", 0.3, "Alpha for exponentially weighted mean (0,1]")
		lastN      = flag.Int("lastN", 10, "Last-N window size for consistency")
		migrations = flag.String("migrations", "./migrations", "Directory with SQL migrations")
		timeout    = flag.Duration("timeout", 30*time.Minute, "Overall timeout for the job")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Ensure DB connection
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	// Apply migrations
	migDir := *migrations
	if env := os.Getenv("MIGRATIONS_DIR"); env != "" {
		migDir = env
	}
	if err := db.RunMigrations(ctx, migDir); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	formatID, err := db.GetMatchFormatIDByCode(ctx, *formatCode)
	if err != nil {
		log.Fatalf("resolve format '%s': %v", *formatCode, err)
	}

	var fromPtr, toPtr *time.Time
	if *fromStr != "" {
		v, err := time.Parse("2006-01-02", *fromStr)
		if err != nil { log.Fatalf("parse -from: %v", err) }
		fromPtr = &v
	}
	if *toStr != "" {
		v, err := time.Parse("2006-01-02", *toStr)
		if err != nil { log.Fatalf("parse -to: %v", err) }
		toPtr = &v
	}

	matches, err := db.ListMatchesByFormatDate(ctx, formatID, fromPtr, toPtr)
	if err != nil {
		log.Fatalf("list matches: %v", err)
	}
	log.Printf("precompute-features: %d matches to process for format %s", len(matches), *formatCode)

	processed := 0
	for _, m := range matches {
		players, err := db.ListPlayersInMatch(ctx, m.MatchID)
		if err != nil {
			log.Fatalf("list players for match %d: %v", m.MatchID, err)
		}
		for _, pid := range players {
			// Batting history strictly before match date
			batHist, err := db.ListBattingBefore(ctx, pid, m.Date, formatID, nil, nil)
			if err != nil { log.Fatalf("bat hist p=%d m=%d: %v", pid, m.MatchID, err) }
			// Bowling history strictly before match date
			bowlHist, err := db.ListBowlingBefore(ctx, pid, m.Date, formatID, nil, nil)
			if err != nil { log.Fatalf("bowl hist p=%d m=%d: %v", pid, m.MatchID, err) }

			// Convert to features.Innings and sort/clip (safety)
			batInn := make([]features.Innings, 0, len(batHist))
			for _, iv := range batHist { batInn = append(batInn, features.Innings{Date: iv.Date, Value: iv.Value}) }
			bowlInn := make([]features.Innings, 0, len(bowlHist))
			for _, iv := range bowlHist { bowlInn = append(bowlInn, features.Innings{Date: iv.Date, Value: iv.Value}) }
			batInn = features.SortAndClip(batInn, m.Date)
			bowlInn = features.SortAndClip(bowlInn, m.Date)

			batForm, effNbat := features.EWM(batInn, *alpha)
			bowlForm, effNbowl := features.EWM(bowlInn, *alpha)
			batCons, nCbat := features.Consistency(batInn, *lastN)
			bowlCons, nCbowl := features.Consistency(bowlInn, *lastN)

			// Persist snapshots (idempotent)
			if err := db.UpsertPlayerFormAsOf(ctx, pid, m.Date, formatID, batForm, bowlForm, effNbat, effNbowl, specEWM(*alpha)); err != nil {
				log.Fatalf("upsert form asof p=%d m=%d: %v", pid, m.MatchID, err)
			}
			if err := db.UpsertPlayerConsistencyAsOf(ctx, pid, m.Date, formatID, batCons, bowlCons, nCbat, nCbowl, specLastN(*lastN)); err != nil {
				log.Fatalf("upsert consistency asof p=%d m=%d: %v", pid, m.MatchID, err)
			}
		}
		processed++
		if processed%50 == 0 {
			log.Printf("processed %d/%d matches...", processed, len(matches))
		}
	}

	log.Printf("done. processed %d matches. snapshots written.", processed)
}

func specEWM(alpha float64) string {
	return "ewm:" + trimFloat(alpha)
}

func specLastN(n int) string {
	return "lastN:" + itoa(n)
}

func trimFloat(f float64) string {
	// Simple trim for logging/spec
	s := fmtFloat(f)
	// remove trailing zeros and dot
	for len(s) > 0 && s[len(s)-1] == '0' { s = s[:len(s)-1] }
	if len(s) > 0 && s[len(s)-1] == '.' { s = s[:len(s)-1] }
	return s
}

func fmtFloat(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }
func itoa(i int) string         { return strconv.Itoa(i) }
