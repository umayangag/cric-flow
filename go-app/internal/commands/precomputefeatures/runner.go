package precomputefeatures

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/features"
	"github.com/umayangag/cric-flow/go-app/internal/resources"
	"github.com/umayangag/cric-flow/go-app/internal/seqcalc"
)

// Runner executes precompute-features workflows.
// It is stateless and delegates to internal/db package helpers for queries and persistence.
type Runner struct{}

func NewRunner() Runner { return Runner{} }

// replayMatchPageSize returns the number of matches to load per chunk (from config or default).
func replayMatchPageSize() int {
	return config.PipelineReplayMatchPageSize(config.Load())
}

// RunReplay iterates through matches chronologically and writes snapshots as of each match date.
// Matches are fetched in chunks to avoid OOM when a format has many matches.
// concurrencyLimit: when > 0, caps concurrent player work for this format (e.g. when multiple formats run in parallel);
// when 0, uses resource-aware limit from config/env (80% of available memory/CPU).
func (Runner) RunReplay(
	ctx context.Context,
	formatCode string,
	formatID int64,
	alpha float64,
	lastN int,
	windowN int,
	concurrencyLimit int,
) error {
	precomputeLimit := resolveConcurrencyLimit(concurrencyLimit)
	pageSize := replayMatchPageSize()
	slog.Info("precompute-features(replay)",
		slog.String("format", formatCode),
		slog.Int("concurrency", precomputeLimit),
		slog.Int("match_page_size", pageSize),
	)
	resources.LogMemoryAndGoroutines(
		"precompute-features(replay): memory and goroutines at start",
		slog.String("format", formatCode),
	)
	// Optional history window from config is provided by caller via windowN.
	processed := int64(0)
	totalMatches := int64(0)
	var after *db.MatchLite
	for {
		matches, err := db.ListMatchesByFormatDatePage(ctx, formatID, nil, nil, pageSize, after)
		if err != nil {
			slog.Error(
				"precompute-features(replay): list matches page failed",
				slog.String("format", formatCode),
				slog.Int64("format_id", formatID),
				slog.Any("err", err),
			)
			return fmt.Errorf("list matches: %w", err)
		}
		if len(matches) == 0 {
			break
		}
		totalMatches += int64(len(matches))
		if totalMatches == int64(len(matches)) {
			slog.Info("pipeline: precompute-features replay processing first page",
				slog.String("format", formatCode),
				slog.Int("matches_in_page", len(matches)))
		}
		for _, m := range matches {
			asOf := m.MatchDate
			players, err := db.ListPlayersInMatch(ctx, m.MatchID)
			if err != nil {
				slog.Error(
					"precompute-features(replay): list players in match failed",
					slog.Int64("match_id", m.MatchID),
					slog.String("format", formatCode),
					slog.Any("err", err),
				)
				return fmt.Errorf("list players in match %d: %w", m.MatchID, err)
			}

			g, pCtx := errgroup.WithContext(ctx)
			g.SetLimit(precomputeLimit)

			for _, pid := range players {
				pid := pid // capture
				g.Go(func() error {
					if err := pCtx.Err(); err != nil {
						return nil
					}
					// Base histories strictly before match date
					batHist, err := db.ListBattingBefore(pCtx, pid, asOf, formatID, nil, nil)
					if err != nil {
						slog.Error(
							"precompute-features(replay): batting history failed",
							slog.Int64("player_id", pid),
							slog.Int64("match_id", m.MatchID),
							slog.String("format", formatCode),
							slog.Any("err", err),
						)
						return fmt.Errorf("batting history pid=%d: %w", pid, err)
					}
					bowlHist, err := db.ListBowlingBefore(pCtx, pid, asOf, formatID, nil, nil)
					if err != nil {
						slog.Error(
							"precompute-features(replay): bowling history failed",
							slog.Int64("player_id", pid),
							slog.Int64("match_id", m.MatchID),
							slog.String("format", formatCode),
							slog.Any("err", err),
						)
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
						slog.Error(
							"precompute-features(replay): upsert form overall failed",
							slog.Int64("player_id", pid),
							slog.Int64("match_id", m.MatchID),
							slog.String("format", formatCode),
							slog.Any("err", err),
						)
						return fmt.Errorf("upsert form overall pid=%d: %w", pid, err)
					}
					if err := db.UpsertFeatureConsistencySnapshot(pCtx, pid, asOf, formatID, "overall", nil,
						batCons, bowlCons, lastN, nCbat, nCbowl, "v1"); err != nil {
						slog.Error(
							"precompute-features(replay): upsert consistency overall failed",
							slog.Int64("player_id", pid),
							slog.Int64("match_id", m.MatchID),
							slog.String("format", formatCode),
							slog.Any("err", err),
						)
						return fmt.Errorf("upsert consistency overall pid=%d: %w", pid, err)
					}

					// opposition specific form
					if m.OppositionID != 0 {
						oppID := m.OppositionID
						oppBat, err := db.ListBattingBefore(pCtx, pid, asOf, formatID, &oppID, nil)
						if err != nil {
							slog.Error(
								"precompute-features(replay): opposition batting history failed",
								slog.Int64("player_id", pid),
								slog.Int64("opposition_id", oppID),
								slog.Int64("match_id", m.MatchID),
								slog.String("format", formatCode),
								slog.Any("err", err),
							)
							return fmt.Errorf("opposition batting history pid=%d opp=%d: %w", pid, oppID, err)
						}
						oppBowl, err := db.ListBowlingBefore(pCtx, pid, asOf, formatID, &oppID, nil)
						if err != nil {
							slog.Error(
								"precompute-features(replay): opposition bowling history failed",
								slog.Int64("player_id", pid),
								slog.Int64("opposition_id", oppID),
								slog.Int64("match_id", m.MatchID),
								slog.String("format", formatCode),
								slog.Any("err", err),
							)
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
							slog.Error(
								"precompute-features(replay): upsert form opposition failed",
								slog.Int64("player_id", pid),
								slog.Int64("opposition_id", oppID),
								slog.Int64("match_id", m.MatchID),
								slog.String("format", formatCode),
								slog.Any("err", err),
							)
							return fmt.Errorf("upsert form opposition pid=%d opp=%d: %w", pid, oppID, err)
						}
					}

					// venue specific form
					if m.VenueID != 0 {
						venueID := m.VenueID
						venBat, err := db.ListBattingBefore(pCtx, pid, asOf, formatID, nil, &venueID)
						if err != nil {
							slog.Error(
								"precompute-features(replay): venue batting history failed",
								slog.Int64("player_id", pid),
								slog.Int64("venue_id", venueID),
								slog.Int64("match_id", m.MatchID),
								slog.String("format", formatCode),
								slog.Any("err", err),
							)
							return fmt.Errorf("venue batting history pid=%d venue=%d: %w", pid, venueID, err)
						}
						venBowl, err := db.ListBowlingBefore(pCtx, pid, asOf, formatID, nil, &venueID)
						if err != nil {
							slog.Error(
								"precompute-features(replay): venue bowling history failed",
								slog.Int64("player_id", pid),
								slog.Int64("venue_id", venueID),
								slog.Int64("match_id", m.MatchID),
								slog.String("format", formatCode),
								slog.Any("err", err),
							)
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
							slog.Error(
								"precompute-features(replay): upsert form venue failed",
								slog.Int64("player_id", pid),
								slog.Int64("venue_id", venueID),
								slog.Int64("match_id", m.MatchID),
								slog.String("format", formatCode),
								slog.Any("err", err),
							)
							return fmt.Errorf("upsert form venue pid=%d venue=%d: %w", pid, venueID, err)
						}
					}
					return nil
				})
			}

			if err := g.Wait(); err != nil {
				slog.Error(
					"precompute-features(replay): errgroup wait failed for match",
					slog.Int64("match_id", m.MatchID),
					slog.String("format", formatCode),
					slog.Any("err", err),
				)
				return err
			}

			newValue := atomic.AddInt64(&processed, int64(len(players)))
			oldValue := newValue - int64(len(players))
			if oldValue/1000 < newValue/1000 {
				resources.LogMemoryAndGoroutines("precompute-features(replay): progress",
					slog.Int64("player_snapshots", newValue),
					slog.String("format", formatCode),
					slog.Int64("match_id", m.MatchID),
				)
			}
		}
		// Next page: cursor after last match in this chunk
		after = &matches[len(matches)-1]
	}

	// Record observed heap/concurrency so next run can use dynamic MB-per-worker (no hardcoded constant).
	resources.RecordWorkerMemorySample(resources.KindPrecompute, precomputeLimit)

	// Trigger sequence features calculation (fill bowling_sequence_features, event_reaction_features, etc.)
	// Run GC to release replay-phase memory before the next heavy phase and reduce OOM risk.
	runtime.GC()
	resources.LogMemoryAndGoroutines(
		"precompute-features(replay): after GC, before seqcalc",
		slog.String("format", formatCode),
	)
	logSeqCalcTrigger(formatCode, "replay")
	if err := triggerSeqCalc(ctx, formatCode, time.Time{}); err != nil {
		slog.Error(
			"precompute-features(replay): sequence calculations failed",
			slog.String("format", formatCode),
			slog.Any("err", err),
		)
		return fmt.Errorf("sequence calculations failed: %w", err)
	}

	slog.Info("done (replay)", slog.Int64("matches", totalMatches), slog.String("format", formatCode))
	return nil
}

// RunPointInTime computes snapshots for all players strictly before the cutoff date concurrently.
// concurrencyLimit: when > 0 use it; when 0 use resource-aware limit (80% of available resources).
func (Runner) RunPointInTime(
	ctx context.Context,
	formatCode string,
	formatID int64,
	asOf time.Time,
	alpha float64,
	lastN int,
	windowN int,
	concurrencyLimit int,
) error {
	players, err := db.ListPlayersWithHistoryBefore(ctx, formatID, asOf)
	if err != nil {
		slog.Error(
			"precompute-features(as-of): list players with history failed",
			slog.String("format", formatCode),
			slog.Int64("format_id", formatID),
			slog.String("as_of", asOf.Format("2006-01-02")),
			slog.Any("err", err),
		)
		return fmt.Errorf("list players with history: %w", err)
	}
	precomputeLimit := resolveConcurrencyLimit(concurrencyLimit)
	slog.Info(
		"precompute-features(as-of)",
		slog.Int("players", len(players)),
		slog.String("format", formatCode),
		slog.String("as_of", asOf.Format("2006-01-02")),
		slog.Int("concurrency", precomputeLimit),
	)
	resources.LogMemoryAndGoroutines("precompute-features(as-of): memory and goroutines at start",
		slog.String("format", formatCode),
		slog.Int("players", len(players)),
	)

	g, pCtx := errgroup.WithContext(ctx)
	g.SetLimit(precomputeLimit)
	var processed int64

	for _, pid := range players {
		pid := pid // capture
		g.Go(func() error {
			if err := pCtx.Err(); err != nil {
				return nil
			}
			batHist, err := db.ListBattingBefore(pCtx, pid, asOf, formatID, nil, nil)
			if err != nil {
				slog.Error(
					"precompute-features(as-of): batting history failed",
					slog.Int64("player_id", pid),
					slog.String("format", formatCode),
					slog.Any("err", err),
				)
				return fmt.Errorf("batting history pid=%d: %w", pid, err)
			}
			bowlHist, err := db.ListBowlingBefore(pCtx, pid, asOf, formatID, nil, nil)
			if err != nil {
				slog.Error(
					"precompute-features(as-of): bowling history failed",
					slog.Int64("player_id", pid),
					slog.String("format", formatCode),
					slog.Any("err", err),
				)
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
				slog.Error(
					"precompute-features(as-of): upsert form overall failed",
					slog.Int64("player_id", pid),
					slog.String("format", formatCode),
					slog.Any("err", err),
				)
				return fmt.Errorf("upsert form overall pid=%d: %w", pid, err)
			}
			if err := db.UpsertFeatureConsistencySnapshot(pCtx, pid, asOf, formatID, "overall", nil, batCons, bowlCons, lastN, nCbat, nCbowl, "v1"); err != nil {
				slog.Error(
					"precompute-features(as-of): upsert consistency overall failed",
					slog.Int64("player_id", pid),
					slog.String("format", formatCode),
					slog.Any("err", err),
				)
				return fmt.Errorf("upsert consistency overall pid=%d: %w", pid, err)
			}
			p := atomic.AddInt64(&processed, 1)
			if p%1000 == 0 {
				resources.LogMemoryAndGoroutines("precompute-features(as-of): progress",
					slog.Int64("players", p),
					slog.String("format", formatCode),
				)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		slog.Error(
			"precompute-features(as-of): errgroup wait failed",
			slog.String("format", formatCode),
			slog.Any("err", err),
		)
		return err
	}
	slog.Info(
		"done (as-of)",
		slog.Int("players", len(players)),
		slog.String("format", formatCode),
		slog.String("as_of", asOf.Format("2006-01-02")),
	)

	resources.RecordWorkerMemorySample(resources.KindPrecompute, precomputeLimit)

	// Trigger sequence features calculation. GC to free form/consistency phase memory before seqcalc.
	runtime.GC()
	resources.LogMemoryAndGoroutines(
		"precompute-features(as-of): after GC, before seqcalc",
		slog.String("format", formatCode),
	)
	logSeqCalcTrigger(formatCode, "as-of")
	if err := triggerSeqCalc(ctx, formatCode, asOf); err != nil {
		slog.Error(
			"precompute-features(as-of): sequence calculations failed",
			slog.String("format", formatCode),
			slog.Any("err", err),
		)
		return fmt.Errorf("sequence calculations failed: %w", err)
	}

	return nil
}

// logSeqCalcTrigger logs concurrency, memory, and goroutine count before triggering sequence calculations (shared by replay and as-of).
func logSeqCalcTrigger(formatCode, mode string) {
	seqcalcLimit := resources.GetLimit(resources.KindSeqCalc)
	resources.LogMemoryAndGoroutines("precompute-features("+mode+"): triggering sequence calculations",
		slog.String("format", formatCode),
		slog.Int("seqcalc_concurrency", seqcalcLimit),
	)
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
		slog.Error(
			"precompute-features: resolve seqcalc targets failed",
			slog.String("format", formatCode),
			slog.Any("err", err),
		)
		return fmt.Errorf("resolve seqcalc targets: %w", err)
	}
	if err := seqcalc.Run(ctxNoDeadline, calcs, seqcalc.Params{FormatCode: formatCode, AsOf: asOf}, false); err != nil {
		slog.Error("precompute-features: seqcalc run failed", slog.String("format", formatCode), slog.Any("err", err))
		return fmt.Errorf("seqcalc run: %w", err)
	}
	return nil
}

// resolveConcurrencyLimit returns the effective concurrency limit: uses requestedLimit when > 0,
// otherwise resources.GetLimit(KindPrecompute), and ensures at least 1.
func resolveConcurrencyLimit(requestedLimit int) int {
	limit := requestedLimit
	if limit <= 0 {
		limit = resources.GetLimit(resources.KindPrecompute)
	}
	if limit < 1 {
		limit = 1
	}
	return limit
}
