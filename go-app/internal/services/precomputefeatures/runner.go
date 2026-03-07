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

// replayWorkItem is one unit of work for the global pool: process one player in one match for one format.
type replayWorkItem struct {
	FormatCode string
	FormatID   int64
	Match      db.MatchLite
	PlayerID   int64
}

// FormatJob identifies a format for global-pool replay (code + format ID).
type FormatJob struct {
	Code     string
	FormatID int64
}

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
	_ float64, // alpha (reserved for future EWM tuning)
	_ int, // lastN (reserved for future last-N window)
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
					return processOnePlayerReplay(pCtx, formatCode, formatID, m, pid, windowN)
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

// RunReplayGlobalPool runs replay for multiple formats using a single shared worker pool.
// Work items (format, match, player) are produced by one producer per format and consumed by totalLimit workers,
// so when one format has no work left, workers take tasks from others instead of sitting idle.
func (Runner) RunReplayGlobalPool(ctx context.Context, jobs []FormatJob, windowN, totalLimit int) error {
	if len(jobs) == 0 {
		return nil
	}
	if totalLimit < 1 {
		totalLimit = 1
	}
	pageSize := replayMatchPageSize()
	slog.Info("precompute-features(replay-global-pool)",
		slog.Int("formats", len(jobs)),
		slog.Int("concurrency", totalLimit),
		slog.Int("match_page_size", pageSize),
	)
	resources.LogMemoryAndGoroutines("precompute-features(replay-global-pool): at start")

	workCh := make(chan *replayWorkItem, totalLimit*2)
	gProducers, prodCtx := errgroup.WithContext(ctx)
	for _, job := range jobs {
		job := job
		gProducers.Go(func() error {
			var after *db.MatchLite
			for {
				if prodCtx.Err() != nil {
					return nil
				}
				matches, err := db.ListMatchesByFormatDatePage(prodCtx, job.FormatID, nil, nil, pageSize, after)
				if err != nil {
					slog.Error("precompute-features(replay-global-pool): list matches failed",
						slog.String("format", job.Code), slog.Int64("format_id", job.FormatID), slog.Any("err", err))
					return fmt.Errorf("list matches %s: %w", job.Code, err)
				}
				if len(matches) == 0 {
					return nil
				}
				for _, m := range matches {
					if prodCtx.Err() != nil {
						return nil
					}
					players, err := db.ListPlayersInMatch(prodCtx, m.MatchID)
					if err != nil {
						slog.Error("precompute-features(replay-global-pool): list players failed",
							slog.Int64("match_id", m.MatchID), slog.String("format", job.Code), slog.Any("err", err))
						return fmt.Errorf("list players match %d: %w", m.MatchID, err)
					}
					for _, pid := range players {
						item := &replayWorkItem{FormatCode: job.Code, FormatID: job.FormatID, Match: m, PlayerID: pid}
						select {
						case workCh <- item:
						case <-prodCtx.Done():
							return nil
						}
					}
				}
				after = &matches[len(matches)-1]
			}
		})
	}
	var producerErr error
	go func() {
		producerErr = gProducers.Wait()
		close(workCh)
	}()

	gWorkers, workCtx := errgroup.WithContext(prodCtx)
	for i := 0; i < totalLimit; i++ {
		gWorkers.Go(func() error {
			for item := range workCh {
				if workCtx.Err() != nil {
					return nil
				}
				if err := processOnePlayerReplay(workCtx, item.FormatCode, item.FormatID, item.Match, item.PlayerID, windowN); err != nil {
					return err
				}
			}
			return nil
		})
	}
	workerErr := gWorkers.Wait()
	if producerErr != nil {
		slog.Error("precompute-features(replay-global-pool): producer error", slog.Any("err", producerErr))
		return producerErr
	}
	if workerErr != nil {
		slog.Error("precompute-features(replay-global-pool): worker error", slog.Any("err", workerErr))
		return workerErr
	}

	resources.RecordWorkerMemorySample(resources.KindPrecompute, totalLimit)
	runtime.GC()
	resources.LogMemoryAndGoroutines("precompute-features(replay-global-pool): after GC, before seqcalc")

	if err := triggerSeqCalcMultiFormat(ctx, jobs, time.Time{}); err != nil {
		slog.Error("precompute-features(replay-global-pool): sequence calculations failed", slog.Any("err", err))
		return err
	}
	slog.Info("precompute-features(replay-global-pool): done", slog.Int("formats", len(jobs)))
	return nil
}

// RunPointInTime computes snapshots for all players strictly before the cutoff date concurrently.
// concurrencyLimit: when > 0 use it; when 0 use resource-aware limit (80% of available resources).
func (Runner) RunPointInTime(
	ctx context.Context,
	formatCode string,
	formatID int64,
	asOf time.Time,
	_ float64, // alpha (reserved for future EWM tuning)
	_ int, // lastN (reserved for future last-N window)
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
			if err := upsertRawStatsForScope(pCtx, pid, asOf, formatID, "overall", nil, batInn, bowlInn, "as-of", 0, formatCode); err != nil {
				return err
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

// logSnapshotUpsertError logs a failed snapshot upsert and returns an error. Used by
// upsertRawStatsForScope to avoid duplicating error-handling logic.
func logSnapshotUpsertError(
	scope string,
	playerID int64,
	snapshotKind string,
	err error,
	mode string,
	formatCode string,
	matchID int64,
) error {
	attrs := []any{
		slog.Int64("player_id", playerID),
		slog.String("scope", scope),
		slog.String("format", formatCode),
		slog.Any("err", err),
	}
	if matchID != 0 {
		attrs = append(attrs, slog.Int64("match_id", matchID))
	}
	slog.Error("precompute-features("+mode+"): upsert "+snapshotKind+" failed", attrs...)
	return fmt.Errorf("upsert %s %s pid=%d: %w", snapshotKind, scope, playerID, err)
}

// upsertRawStatsForScope computes WindowedStats from the given bat/bowl innings and upserts
// raw stats snapshots for the given scope. Form/consistency snapshots are no longer written;
// raw stats replace them for ML. mode is "replay" or "as-of" for logging; matchID should be 0 for point-in-time runs.
func upsertRawStatsForScope(
	ctx context.Context,
	playerID int64,
	asOf time.Time,
	formatID int64,
	scope string,
	scopeID *int64,
	batInn, bowlInn []features.Innings,
	mode string,
	matchID int64,
	formatCode string,
) error {
	batRaw := features.WindowedStats(batInn, asOf)
	bowlRaw := features.WindowedStats(bowlInn, asOf)
	if err := db.UpsertFeatureRawStatsSnapshot(ctx, playerID, asOf, formatID, scope, scopeID, batRaw, bowlRaw, features.ContractVersion()); err != nil {
		return logSnapshotUpsertError(scope, playerID, "raw stats", err, mode, formatCode, matchID)
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

// triggerSeqCalcMultiFormat runs sequence calculators for multiple formats using a single shared worker pool.
func triggerSeqCalcMultiFormat(ctx context.Context, jobs []FormatJob, asOf time.Time) error {
	if len(jobs) == 0 {
		return nil
	}
	ctxNoDeadline, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-ctx.Done()
		cancel()
	}()

	reg := seqcalc.NewDefaultRegistry()
	calcs, err := reg.ResolveTargets("all")
	if err != nil {
		slog.Error("precompute-features: resolve seqcalc targets failed (multi-format)", slog.Any("err", err))
		return fmt.Errorf("resolve seqcalc targets: %w", err)
	}
	paramsList := make([]seqcalc.Params, 0, len(jobs))
	for _, j := range jobs {
		paramsList = append(paramsList, seqcalc.Params{FormatCode: j.Code, AsOf: asOf})
	}
	limit := resources.GetLimit(resources.KindSeqCalc)
	if limit < 1 {
		limit = 1
	}
	logSeqCalcTrigger(jobs[0].Code, "replay") // log once with first format for observability
	if err := seqcalc.RunMultiFormat(ctxNoDeadline, calcs, paramsList, false, limit); err != nil {
		return fmt.Errorf("seqcalc multi-format run: %w", err)
	}
	return nil
}

// processOnePlayerReplay loads history, computes raw stats, and upserts for one player in one match (overall, opposition, venue scopes).
func processOnePlayerReplay(
	ctx context.Context,
	formatCode string,
	formatID int64,
	m db.MatchLite,
	playerID int64,
	windowN int,
) error {
	if err := ctx.Err(); err != nil {
		return nil
	}

	type replayScope struct {
		name         string
		oppositionID *int64
		venueID      *int64
		scopeID      *int64
	}

	scopes := []replayScope{
		{name: "overall"},
	}
	if m.OppositionID != 0 {
		oppID := m.OppositionID
		scopes = append(scopes, replayScope{name: "opposition", oppositionID: &oppID, scopeID: &oppID})
	}
	if m.VenueID != 0 {
		venueID := m.VenueID
		scopes = append(scopes, replayScope{name: "venue", venueID: &venueID, scopeID: &venueID})
	}

	asOf := m.MatchDate
	for _, scope := range scopes {
		batHist, err := db.ListBattingBefore(ctx, playerID, asOf, formatID, scope.oppositionID, scope.venueID)
		if err != nil {
			slog.Error(
				"precompute-features(replay): batting history failed",
				slog.String(
					"scope",
					scope.name,
				),
				slog.Int64("player_id", playerID),
				slog.Int64("match_id", m.MatchID),
				slog.String("format", formatCode),
				slog.Any("err", err),
			)
			return fmt.Errorf("%s batting history pid=%d: %w", scope.name, playerID, err)
		}
		bowlHist, err := db.ListBowlingBefore(ctx, playerID, asOf, formatID, scope.oppositionID, scope.venueID)
		if err != nil {
			slog.Error(
				"precompute-features(replay): bowling history failed",
				slog.String(
					"scope",
					scope.name,
				),
				slog.Int64("player_id", playerID),
				slog.Int64("match_id", m.MatchID),
				slog.String("format", formatCode),
				slog.Any("err", err),
			)
			return fmt.Errorf("%s bowling history pid=%d: %w", scope.name, playerID, err)
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
		if err := upsertRawStatsForScope(ctx, playerID, asOf, formatID, scope.name, scope.scopeID, batInn, bowlInn, "replay", m.MatchID, formatCode); err != nil {
			return err
		}
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
