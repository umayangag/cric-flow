package precomputefeatures

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

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
	processed := int64(0)
	for _, m := range matches {
		asOf := m.MatchDate
		players, err := db.ListPlayersInMatch(ctx, m.MatchID)
		if err != nil {
			return fmt.Errorf("list players in match %d: %w", m.MatchID, err)
		}

		g, pCtx := errgroup.WithContext(ctx)
		g.SetLimit(runtime.NumCPU())

		for _, pid := range players {
			pid := pid // capture
			g.Go(func() error {
				if err := pCtx.Err(); err != nil {
					return nil
				}
				// Base histories strictly before match date
				batHist, err := db.ListBattingBefore(pCtx, pid, asOf, formatID, nil, nil)
				if err != nil {
					return fmt.Errorf("batting history pid=%d: %w", pid, err)
				}
				bowlHist, err := db.ListBowlingBefore(pCtx, pid, asOf, formatID, nil, nil)
				if err != nil {
					return fmt.Errorf("bowling history pid=%d: %w", pid, err)
				}

				batInn := toFeatureInnings(batHist)
				bowlInn := toFeatureInnings(bowlHist)
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

				if err := db.UpsertFeatureFormSnapshot(pCtx, pid, asOf, formatID, "overall", nil,
					batForm, bowlForm, alpha, effNbat, effNbowl, effNbat+effNbowl, "v1"); err != nil {
					return fmt.Errorf("upsert form overall pid=%d: %w", pid, err)
				}
				if err := db.UpsertFeatureConsistencySnapshot(pCtx, pid, asOf, formatID, "overall", nil,
					batCons, bowlCons, lastN, nCbat, nCbowl, "v1"); err != nil {
					return fmt.Errorf("upsert consistency overall pid=%d: %w", pid, err)
				}

				// opposition specific form
				if m.OppositionID != 0 {
					oppID := m.OppositionID
					oppBat, err := db.ListBattingBefore(pCtx, pid, asOf, formatID, &oppID, nil)
					if err != nil {
						return fmt.Errorf("opposition batting history pid=%d opp=%d: %w", pid, oppID, err)
					}
					oppBowl, err := db.ListBowlingBefore(pCtx, pid, asOf, formatID, &oppID, nil)
					if err != nil {
						return fmt.Errorf("opposition bowling history pid=%d opp=%d: %w", pid, oppID, err)
					}
					oppBatInn := toFeatureInnings(oppBat)
					oppBowlInn := toFeatureInnings(oppBowl)
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
					if err := db.UpsertFeatureFormSnapshot(pCtx, pid, asOf, formatID, "opposition", &oppID,
						oppBatForm, oppBowlForm, alpha, nOppBat, nOppBowl, nOppBat+nOppBowl, "v1"); err != nil {
						return fmt.Errorf("upsert form opposition pid=%d opp=%d: %w", pid, oppID, err)
					}
				}

				// venue specific form
				if m.VenueID != 0 {
					venueID := m.VenueID
					venBat, err := db.ListBattingBefore(pCtx, pid, asOf, formatID, nil, &venueID)
					if err != nil {
						return fmt.Errorf("venue batting history pid=%d venue=%d: %w", pid, venueID, err)
					}
					venBowl, err := db.ListBowlingBefore(pCtx, pid, asOf, formatID, nil, &venueID)
					if err != nil {
						return fmt.Errorf("venue bowling history pid=%d venue=%d: %w", pid, venueID, err)
					}
					venBatInn := toFeatureInnings(venBat)
					venBowlInn := toFeatureInnings(venBowl)
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
					if err := db.UpsertFeatureFormSnapshot(pCtx, pid, asOf, formatID, "venue", &venueID,
						venBatForm, venBowlForm, alpha, nVenBat, nVenBowl, nVenBat+nVenBowl, "v1"); err != nil {
						return fmt.Errorf("upsert form venue pid=%d venue=%d: %w", pid, venueID, err)
					}
				}
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}

		newValue := atomic.AddInt64(&processed, int64(len(players)))
		oldValue := newValue - int64(len(players))
		if oldValue/1000 < newValue/1000 {
			slog.Info("progress", slog.Int64("player_snapshots", newValue), slog.String("format", formatCode))
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

// RunPointInTime computes snapshots for all players strictly before the cutoff date concurrently.
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

	g, pCtx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())
	var processed int64

	for _, pid := range players {
		pid := pid // capture
		g.Go(func() error {
			if err := pCtx.Err(); err != nil {
				return nil
			}
			batHist, err := db.ListBattingBefore(pCtx, pid, asOf, formatID, nil, nil)
			if err != nil {
				return fmt.Errorf("batting history pid=%d: %w", pid, err)
			}
			bowlHist, err := db.ListBowlingBefore(pCtx, pid, asOf, formatID, nil, nil)
			if err != nil {
				return fmt.Errorf("bowling history pid=%d: %w", pid, err)
			}
			batInn := toFeatureInnings(batHist)
			bowlInn := toFeatureInnings(bowlHist)
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
			if err := db.UpsertFeatureFormSnapshot(pCtx, pid, asOf, formatID, "overall", nil, batForm, bowlForm, alpha, effNbat, effNbowl, effNbat+effNbowl, "v1"); err != nil {
				return fmt.Errorf("upsert form overall pid=%d: %w", pid, err)
			}
			if err := db.UpsertFeatureConsistencySnapshot(pCtx, pid, asOf, formatID, "overall", nil, batCons, bowlCons, lastN, nCbat, nCbowl, "v1"); err != nil {
				return fmt.Errorf("upsert consistency overall pid=%d: %w", pid, err)
			}
			p := atomic.AddInt64(&processed, 1)
			if p%1000 == 0 {
				slog.Info("progress", slog.Int64("players", p), slog.String("format", formatCode))
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
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

func toFeatureInnings(in []db.InnVal) []features.Innings {
	out := make([]features.Innings, 0, len(in))
	for _, iv := range in {
		out = append(out, features.Innings{Date: iv.MatchDate, Value: iv.Value})
	}
	return out
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
