// Command precompute-features replays matches chronologically and persists
// leakage-free, date-indexed (as-of) feature snapshots per player.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
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

	logger.SetupFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Ensure DB connection
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}

	// Apply migrations
	migDir := *migrations
	if env := os.Getenv("MIGRATIONS_DIR"); env != "" {
		migDir = env
	}
 if err := db.RunMigrations(ctx, migDir); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		os.Exit(1)
	}

	formatID, err := db.GetMatchFormatIDByCode(ctx, *formatCode)
	if err != nil {
		slog.Error("resolve format failed", slog.String("format", *formatCode), slog.Any("err", err))
		os.Exit(1)
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
			slog.Error("list matches failed", slog.Any("err", err))
			os.Exit(1)
		}
		slog.Info("precompute-features(replay)", slog.Int("matches", len(matches)), slog.String("format", *formatCode))
		processed := 0
		for _, m := range matches {
			asOf := m.Date
   players, err := db.ListPlayersInMatch(ctx, m.MatchID)
			if err != nil {
				slog.Error("list players in match failed", slog.Int64("match_id", m.MatchID), slog.Any("err", err))
				os.Exit(1)
			}
			for _, pid := range players {
				// Base histories strictly before match date
				batHist, err := db.ListBattingBefore(ctx, pid, asOf, formatID, nil, nil)
				if err != nil {
					slog.Error("batting history query failed", slog.Int64("player_id", pid), slog.Any("err", err))
					os.Exit(1)
				}
				bowlHist, err := db.ListBowlingBefore(ctx, pid, asOf, formatID, nil, nil)
				if err != nil {
					slog.Error("bowling history query failed", slog.Int64("player_id", pid), slog.Any("err", err))
					os.Exit(1)
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

    // Write overall snapshots (form + consistency)
				if err := db.UpsertFeatureFormSnapshot(ctx, pid, asOf, formatID, "overall", nil,
					batForm, bowlForm, *alpha, effNbat, effNbowl, effNbat+effNbowl, "v1"); err != nil {
					slog.Error("upsert feature_form overall failed", slog.Int64("player_id", pid), slog.Any("err", err))
					os.Exit(1)
				}
				if err := db.UpsertFeatureConsistencySnapshot(ctx, pid, asOf, formatID, "overall", nil,
					batCons, bowlCons, *lastN, nCbat, nCbowl, "v1"); err != nil {
					slog.Error("upsert feature_consistency overall failed", slog.Int64("player_id", pid), slog.Any("err", err))
					os.Exit(1)
				}

				// Vs-opposition and at-venue filtered histories (form only, to preserve current behavior)
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
     if err := db.UpsertFeatureFormSnapshot(ctx, pid, asOf, formatID, "opposition", &oppID,
						oppBatForm, oppBowlForm, *alpha, nOppBat, nOppBowl, nOppBat+nOppBowl, "v1"); err != nil {
						slog.Error("upsert feature_form opposition failed", slog.Int64("player_id", pid), slog.Int64("opp_id", oppID), slog.Any("err", err))
						os.Exit(1)
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
     if err := db.UpsertFeatureFormSnapshot(ctx, pid, asOf, formatID, "venue", &venueID,
						venBatForm, venBowlForm, *alpha, nVenBat, nVenBowl, nVenBat+nVenBowl, "v1"); err != nil {
						slog.Error("upsert feature_form venue failed", slog.Int64("player_id", pid), slog.Int64("venue_id", venueID), slog.Any("err", err))
						os.Exit(1)
					}
				}
			}
			processed += len(players)
   if processed%1000 == 0 {
				slog.Info("progress", slog.Int("player_snapshots", processed), slog.String("format", *formatCode))
			}
		}
		slog.Info("done (replay)", slog.Int("matches", len(matches)), slog.String("format", *formatCode))
		return
	}

	// Single-date mode (as-of)
	var asOf time.Time
	if asOfStr == nil || *asOfStr == "" {
		// Default to today's date in UTC when -as-of is not provided
		asOf = time.Now().UTC()
		slog.Info("no -as-of provided; defaulting to today (UTC)", slog.String("as_of", asOf.Format("2006-01-02")))
	} else {
		var err error
		asOf, err = time.Parse("2006-01-02", *asOfStr)
		if err != nil {
			slog.Error("parse -as-of failed", slog.Any("err", err))
			os.Exit(1)
		}
	}

	// Determine players who have any history before the cutoff in this format
	players, err := db.ListPlayersWithHistoryBefore(ctx, formatID, asOf)
	if err != nil {
		slog.Error("list players with history failed", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info(
		"precompute-features(as-of)",
		slog.Int("players", len(players)),
		slog.String("format", *formatCode),
		slog.String("as_of", asOf.Format("2006-01-02")),
	)

	processed := 0
	for _, pid := range players {
		// Batting history strictly before as-of
  batHist, err := db.ListBattingBefore(ctx, pid, asOf, formatID, nil, nil)
		if err != nil {
			slog.Error("batting history query failed", slog.Int64("player_id", pid), slog.Any("err", err))
			os.Exit(1)
		}
		bowlHist, err := db.ListBowlingBefore(ctx, pid, asOf, formatID, nil, nil)
		if err != nil {
			slog.Error("bowling history query failed", slog.Int64("player_id", pid), slog.Any("err", err))
			os.Exit(1)
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

		if err := db.UpsertFeatureFormSnapshot(ctx, pid, asOf, formatID, "overall", nil,
			batForm, bowlForm, *alpha, effNbat, effNbowl, effNbat+effNbowl, "v1"); err != nil {
			slog.Error("upsert feature_form overall failed", slog.Int64("player_id", pid), slog.Any("err", err))
			os.Exit(1)
		}
		if err := db.UpsertFeatureConsistencySnapshot(ctx, pid, asOf, formatID, "overall", nil,
			batCons, bowlCons, *lastN, nCbat, nCbowl, "v1"); err != nil {
			slog.Error("upsert feature_consistency overall failed", slog.Int64("player_id", pid), slog.Any("err", err))
			os.Exit(1)
		}

		processed++
		if processed%1000 == 0 {
			slog.Info("progress", slog.Int("players", processed), slog.String("format", *formatCode))
		}
	}
	slog.Info(
		"done (as-of)",
		slog.Int("players", len(players)),
		slog.String("format", *formatCode),
		slog.String("as_of", asOf.Format("2006-01-02")),
	)
}
