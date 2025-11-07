// Command precompute-features replays matches chronologically and persists
// leakage-free, date-indexed (as-of) feature snapshots per player.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
)

func main() {
	var (
		formatCode = flag.String("format", "ODI", "Match format code: TEST|ODI|T20|T20I")
		asOfStr    = flag.String(
			"as-of",
			"",
			"Cutoff date (YYYY-MM-DD). Snapshots are computed using only matches strictly before this date.",
		)
		replay = flag.Bool(
			"replay",
			false,
			"Replay mode: iterate matches chronologically and write snapshots as of each match date (ignores -as-of)",
		)
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

	// Read optional history window from config
	cfg := config.Load()
	windowN := 0
	if cfg != nil && cfg.Features.HistoryWindowMatches > 0 {
		windowN = cfg.Features.HistoryWindowMatches
	}

	if *replay {
		// Chronological replay: for each match date, compute snapshots as of that date
		matches, err := db.ListMatchesByFormatDate(ctx, formatID, nil, nil)
		if err != nil {
			log.Fatalf("list matches: %v", err)
		}
		log.Printf("precompute-features(replay): %d matches for format %s", len(matches), *formatCode)
		processed := 0
		for _, m := range matches {
			asOf := m.Date
			players, err := db.ListPlayersInMatch(ctx, m.MatchID)
			if err != nil {
				log.Fatalf("list players in match %d: %v", m.MatchID, err)
			}
			for _, pid := range players {
				// Base histories strictly before match date
				batHist, err := db.ListBattingBefore(ctx, pid, asOf, formatID, nil, nil)
				if err != nil {
					log.Fatalf("bat hist p=%d: %v", pid, err)
				}
				bowlHist, err := db.ListBowlingBefore(ctx, pid, asOf, formatID, nil, nil)
				if err != nil {
					log.Fatalf("bowl hist p=%d: %v", pid, err)
				}

				batInn := make([]features.Innings, 0, len(batHist))
				for _, iv := range batHist {
					batInn = append(batInn, features.Innings{Date: iv.Date, Value: iv.Value})
				}
				bowlInn := make([]features.Innings, 0, len(bowlHist))
				for _, iv := range bowlHist {
					bowlInn = append(bowlInn, features.Innings{Date: iv.Date, Value: iv.Value})
				}
				batInn = features.SortAndClip(batInn, asOf)
				bowlInn = features.SortAndClip(bowlInn, asOf)
				if windowN > 0 {
					if len(batInn) > windowN {
						batInn = batInn[len(batInn)-windowN:]
					}
					if len(bowlInn) > windowN {
						bowlInn = bowlInn[len(bowlInn)-windowN:]
					}
				}

				batForm, effNbat := features.EWM(batInn, *alpha)
				bowlForm, effNbowl := features.EWM(bowlInn, *alpha)
				batCons, nCbat := features.Consistency(batInn, *lastN)
				bowlCons, nCbowl := features.Consistency(bowlInn, *lastN)

				if err := db.UpsertPlayerFormAsOf(ctx, pid, asOf, formatID, batForm, bowlForm, effNbat, effNbowl, specEWM(*alpha)); err != nil {
					log.Fatalf("upsert form asof p=%d: %v", pid, err)
				}
				if err := db.UpsertPlayerConsistencyAsOf(ctx, pid, asOf, formatID, batCons, bowlCons, nCbat, nCbowl, specLastN(*lastN)); err != nil {
					log.Fatalf("upsert consistency asof p=%d: %v", pid, err)
				}

				// Vs-opposition and at-venue filtered histories
				if m.OppositionID != 0 {
					oppID := m.OppositionID
					oppBat, _ := db.ListBattingBefore(ctx, pid, asOf, formatID, &oppID, nil)
					oppBowl, _ := db.ListBowlingBefore(ctx, pid, asOf, formatID, &oppID, nil)
					oppBatInn := make([]features.Innings, 0, len(oppBat))
					for _, iv := range oppBat {
						oppBatInn = append(oppBatInn, features.Innings{Date: iv.Date, Value: iv.Value})
					}
					oppBowlInn := make([]features.Innings, 0, len(oppBowl))
					for _, iv := range oppBowl {
						oppBowlInn = append(oppBowlInn, features.Innings{Date: iv.Date, Value: iv.Value})
					}
					oppBatInn = features.SortAndClip(oppBatInn, asOf)
					oppBowlInn = features.SortAndClip(oppBowlInn, asOf)
					if windowN > 0 {
						if len(oppBatInn) > windowN {
							oppBatInn = oppBatInn[len(oppBatInn)-windowN:]
						}
						if len(oppBowlInn) > windowN {
							oppBowlInn = oppBowlInn[len(oppBowlInn)-windowN:]
						}
					}
					oppBatForm, nOppBat := features.EWM(oppBatInn, *alpha)
					oppBowlForm, nOppBowl := features.EWM(oppBowlInn, *alpha)
					if err := db.UpsertPlayerVsOppAsOf(ctx, pid, oppID, asOf, formatID, oppBatForm, oppBowlForm, int(nOppBat+nOppBowl), specEWM(*alpha)); err != nil {
						log.Fatalf("upsert vs_opp asof p=%d: %v", pid, err)
					}
				}
				if m.VenueID != 0 {
					venueID := m.VenueID
					venBat, _ := db.ListBattingBefore(ctx, pid, asOf, formatID, nil, &venueID)
					venBowl, _ := db.ListBowlingBefore(ctx, pid, asOf, formatID, nil, &venueID)
					venBatInn := make([]features.Innings, 0, len(venBat))
					for _, iv := range venBat {
						venBatInn = append(venBatInn, features.Innings{Date: iv.Date, Value: iv.Value})
					}
					venBowlInn := make([]features.Innings, 0, len(venBowl))
					for _, iv := range venBowl {
						venBowlInn = append(venBowlInn, features.Innings{Date: iv.Date, Value: iv.Value})
					}
					venBatInn = features.SortAndClip(venBatInn, asOf)
					venBowlInn = features.SortAndClip(venBowlInn, asOf)
					if windowN > 0 {
						if len(venBatInn) > windowN {
							venBatInn = venBatInn[len(venBatInn)-windowN:]
						}
						if len(venBowlInn) > windowN {
							venBowlInn = venBowlInn[len(venBowlInn)-windowN:]
						}
					}
					venBatForm, nVenBat := features.EWM(venBatInn, *alpha)
					venBowlForm, nVenBowl := features.EWM(venBowlInn, *alpha)
					if err := db.UpsertPlayerAtVenueAsOf(ctx, pid, venueID, asOf, formatID, venBatForm, venBowlForm, int(nVenBat+nVenBowl), specEWM(*alpha)); err != nil {
						log.Fatalf("upsert at_venue asof p=%d: %v", pid, err)
					}
				}
			}
			processed += len(players)
			if processed%1000 == 0 {
				log.Printf("processed %d player-snapshots so far for %s", processed, *formatCode)
			}
		}
		log.Printf("done: replay snapshots computed for %d matches (%s)", len(matches), *formatCode)
		return
	}

	// Single-date mode (as-of)
	var asOf time.Time
	if asOfStr == nil || *asOfStr == "" {
		// Default to today's date in UTC when -as-of is not provided
		asOf = time.Now().UTC()
		log.Printf("no -as-of provided; defaulting to today's date (UTC): %s", asOf.Format("2006-01-02"))
	} else {
		var err error
		asOf, err = time.Parse("2006-01-02", *asOfStr)
		if err != nil {
			log.Fatalf("parse -as-of: %v", err)
		}
	}

	// Determine players who have any history before the cutoff in this format
	players, err := db.ListPlayersWithHistoryBefore(ctx, formatID, asOf)
	if err != nil {
		log.Fatalf("list players with history: %v", err)
	}
	log.Printf(
		"precompute-features(as-of): %d players to process for format %s at %s",
		len(players),
		*formatCode,
		asOf.Format("2006-01-02"),
	)

	processed := 0
	for _, pid := range players {
		// Batting history strictly before as-of
		batHist, err := db.ListBattingBefore(ctx, pid, asOf, formatID, nil, nil)
		if err != nil {
			log.Fatalf("bat hist p=%d: %v", pid, err)
		}
		// Bowling history strictly before as-of
		bowlHist, err := db.ListBowlingBefore(ctx, pid, asOf, formatID, nil, nil)
		if err != nil {
			log.Fatalf("bowl hist p=%d: %v", pid, err)
		}

		// Convert to features.Innings and sort/clip
		batInn := make([]features.Innings, 0, len(batHist))
		for _, iv := range batHist {
			batInn = append(batInn, features.Innings{Date: iv.Date, Value: iv.Value})
		}
		bowlInn := make([]features.Innings, 0, len(bowlHist))
		for _, iv := range bowlHist {
			bowlInn = append(bowlInn, features.Innings{Date: iv.Date, Value: iv.Value})
		}
		batInn = features.SortAndClip(batInn, asOf)
		bowlInn = features.SortAndClip(bowlInn, asOf)

		// Apply optional window from config (last K matches)
		if windowN > 0 {
			if len(batInn) > windowN {
				batInn = batInn[len(batInn)-windowN:]
			}
			if len(bowlInn) > windowN {
				bowlInn = bowlInn[len(bowlInn)-windowN:]
			}
		}

		batForm, effNbat := features.EWM(batInn, *alpha)
		bowlForm, effNbowl := features.EWM(bowlInn, *alpha)
		batCons, nCbat := features.Consistency(batInn, *lastN)
		bowlCons, nCbowl := features.Consistency(bowlInn, *lastN)

		// Persist snapshots (idempotent)
		if err := db.UpsertPlayerFormAsOf(ctx, pid, asOf, formatID, batForm, bowlForm, effNbat, effNbowl, specEWM(*alpha)); err != nil {
			log.Fatalf("upsert form asof p=%d: %v", pid, err)
		}
		if err := db.UpsertPlayerConsistencyAsOf(ctx, pid, asOf, formatID, batCons, bowlCons, nCbat, nCbowl, specLastN(*lastN)); err != nil {
			log.Fatalf("upsert consistency asof p=%d: %v", pid, err)
		}

		processed++
		if processed%1000 == 0 {
			log.Printf("processed %d/%d players for %s", processed, len(players), *formatCode)
		}
	}
	log.Printf("done: %d player snapshots upserted for %s at %s", processed, *formatCode, asOf.Format("2006-01-02"))
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
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

func fmtFloat(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }
func itoa(i int) string         { return strconv.Itoa(i) }
