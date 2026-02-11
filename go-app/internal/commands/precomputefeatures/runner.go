package precomputefeatures

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/seqcalc"
)

// Runner executes precompute-features workflows.
// It is stateless and delegates to internal/db package helpers for queries and persistence.
type Runner struct{}

func NewRunner() Runner { return Runner{} }

// RunReplay iterates through matches chronologically and writes snapshots as of each match date.
// Behavior mirrors the previous cmd implementation.
func (Runner) RunReplay(
	ctx context.Context,
	formatCode string,
	formatID int64,
	alpha float64,
	lastN int,
	windowN int,
) error {
	matches, err := db.ListMatchesByFormatDate(ctx, formatID, nil, nil)
	if err != nil {
		return fmt.Errorf("list matches: %w", err)
	}
	slog.Info("precompute-features(replay)", slog.Int("matches", len(matches)), slog.String("format", formatCode))
	// Optional history window from config is provided by caller via windowN.
	processed := 0
	for _, m := range matches {
		asOf := m.Date
		players, err := db.ListPlayersInMatch(ctx, m.MatchID)
		if err != nil {
			return fmt.Errorf("list players in match %d: %w", m.MatchID, err)
		}
		for _, pid := range players {
			// Base histories strictly before match date
			batHist, err := db.ListBattingBefore(ctx, pid, asOf, formatID, nil, nil)
			if err != nil {
				return fmt.Errorf("batting history pid=%d: %w", pid, err)
			}
			bowlHist, err := db.ListBowlingBefore(ctx, pid, asOf, formatID, nil, nil)
			if err != nil {
				return fmt.Errorf("bowling history pid=%d: %w", pid, err)
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

			batForm, effNbat := features.EWM(batInn, alpha)
			bowlForm, effNbowl := features.EWM(bowlInn, alpha)
			batCons, nCbat := features.Consistency(batInn, lastN)
			bowlCons, nCbowl := features.Consistency(bowlInn, lastN)

			if err := db.UpsertFeatureFormSnapshot(ctx, pid, asOf, formatID, "overall", nil,
				batForm, bowlForm, alpha, effNbat, effNbowl, effNbat+effNbowl, "v1"); err != nil {
				return fmt.Errorf("upsert form overall pid=%d: %w", pid, err)
			}
			if err := db.UpsertFeatureConsistencySnapshot(ctx, pid, asOf, formatID, "overall", nil,
				batCons, bowlCons, lastN, nCbat, nCbowl, "v1"); err != nil {
				return fmt.Errorf("upsert consistency overall pid=%d: %w", pid, err)
			}

			// opposition specific form
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
				oppBatForm, nOppBat := features.EWM(oppBatInn, alpha)
				oppBowlForm, nOppBowl := features.EWM(oppBowlInn, alpha)
				if err := db.UpsertFeatureFormSnapshot(ctx, pid, asOf, formatID, "opposition", &oppID,
					oppBatForm, oppBowlForm, alpha, nOppBat, nOppBowl, nOppBat+nOppBowl, "v1"); err != nil {
					return fmt.Errorf("upsert form opposition pid=%d opp=%d: %w", pid, oppID, err)
				}
			}

			// venue specific form
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
				venBatForm, nVenBat := features.EWM(venBatInn, alpha)
				venBowlForm, nVenBowl := features.EWM(venBowlInn, alpha)
				if err := db.UpsertFeatureFormSnapshot(ctx, pid, asOf, formatID, "venue", &venueID,
					venBatForm, venBowlForm, alpha, nVenBat, nVenBowl, nVenBat+nVenBowl, "v1"); err != nil {
					return fmt.Errorf("upsert form venue pid=%d venue=%d: %w", pid, venueID, err)
				}
			}
		}
		processed += len(players)
		if processed%1000 == 0 {
			slog.Info("progress", slog.Int("player_snapshots", processed), slog.String("format", formatCode))
		}
	}

	// Trigger sequence features calculation (fill bowling_sequence_features, event_reaction_features, etc.)
	slog.Info("precompute-features(replay): triggering sequence calculations", slog.String("format", formatCode))
	if err := triggerSeqCalc(ctx, formatCode, time.Time{}); err != nil {
		return fmt.Errorf("sequence calculations failed: %w", err)
	}

	slog.Info("done (replay)", slog.Int("matches", len(matches)), slog.String("format", formatCode))
	return nil
}

// RunPointInTime computes snapshots for all players strictly before the cutoff date.
func (Runner) RunPointInTime(
	ctx context.Context,
	formatCode string,
	formatID int64,
	asOf time.Time,
	alpha float64,
	lastN int,
	windowN int,
) error {
	players, err := db.ListPlayersWithHistoryBefore(ctx, formatID, asOf)
	if err != nil {
		return fmt.Errorf("list players with history: %w", err)
	}
	slog.Info(
		"precompute-features(as-of)",
		slog.Int("players", len(players)),
		slog.String("format", formatCode),
		slog.String("as_of", asOf.Format("2006-01-02")),
	)
	processed := 0
	for _, pid := range players {
		batHist, err := db.ListBattingBefore(ctx, pid, asOf, formatID, nil, nil)
		if err != nil {
			return fmt.Errorf("batting history pid=%d: %w", pid, err)
		}
		bowlHist, err := db.ListBowlingBefore(ctx, pid, asOf, formatID, nil, nil)
		if err != nil {
			return fmt.Errorf("bowling history pid=%d: %w", pid, err)
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
		batForm, effNbat := features.EWM(batInn, alpha)
		bowlForm, effNbowl := features.EWM(bowlInn, alpha)
		batCons, nCbat := features.Consistency(batInn, lastN)
		bowlCons, nCbowl := features.Consistency(bowlInn, lastN)
		if err := db.UpsertFeatureFormSnapshot(ctx, pid, asOf, formatID, "overall", nil, batForm, bowlForm, alpha, effNbat, effNbowl, effNbat+effNbowl, "v1"); err != nil {
			return fmt.Errorf("upsert form overall pid=%d: %w", pid, err)
		}
		if err := db.UpsertFeatureConsistencySnapshot(ctx, pid, asOf, formatID, "overall", nil, batCons, bowlCons, lastN, nCbat, nCbowl, "v1"); err != nil {
			return fmt.Errorf("upsert consistency overall pid=%d: %w", pid, err)
		}
		processed++
		if processed%1000 == 0 {
			slog.Info("progress", slog.Int("players", processed), slog.String("format", formatCode))
		}
	}
	slog.Info(
		"done (as-of)",
		slog.Int("players", len(players)),
		slog.String("format", formatCode),
		slog.String("as_of", asOf.Format("2006-01-02")),
	)

	// Trigger sequence features calculation
	slog.Info("precompute-features(as-of): triggering sequence calculations", slog.String("format", formatCode))
	if err := triggerSeqCalc(ctx, formatCode, asOf); err != nil {
		return fmt.Errorf("sequence calculations failed: %w", err)
	}

	return nil
}

func triggerSeqCalc(ctx context.Context, formatCode string, asOf time.Time) error {
	// Sequence calculations can take significantly longer than the overall
	// precompute timeout. Allow them to run without being bound to the
	// caller's deadline. We still want to cancel if the parent context is
	// explicitly canceled for other reasons, so we forward cancellation.
	ctxNoDeadline, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-ctx.Done()
		cancel()
	}()

	reg := seqcalc.NewDefaultRegistry()
	calcs, err := reg.ResolveTargets("all")
	if err != nil {
		return fmt.Errorf("resolve seqcalc targets: %w", err)
	}
	if err := seqcalc.Run(ctxNoDeadline, calcs, seqcalc.Params{FormatCode: formatCode, AsOf: asOf}, false); err != nil {
		return fmt.Errorf("seqcalc run: %w", err)
	}
	return nil
}
