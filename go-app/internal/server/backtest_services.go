package server

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
	"golang.org/x/sync/errgroup"
)

const accuracyTrendConcurrency = 8

type accuracyTrendCandidatePrep struct {
	cutoff  time.Time
	squad   []int64
	actuals map[int64]playerActuals
	format  string
}

// resolveCandidateCutoff determines the cutoff timestamp for a backtest candidate.
// Preference order:
// 1) Database match date via seam getBacktestMatchDateFunc
// 2) Fallback to parsing the candidate's Date (RFC3339)
// Returns zero time if both sources are unavailable/invalid.
func resolveCandidateCutoff(ctx context.Context, m backtestCandidate) time.Time {
	if cutoff, err := getBacktestMatchDateFunc(ctx, m.MatchID); err == nil && !cutoff.IsZero() {
		return cutoff
	}
	if t, err := time.Parse(time.RFC3339, m.MatchDate); err == nil {
		return t
	}
	return time.Time{}
}

// getPredictedAggregates encapsulates cache read/write and ML call for match/team aggregates.
// It returns the predicted aggregates and a boolean indicating whether a prediction is available.
func getPredictedAggregates(
	ctx context.Context,
	m backtestCandidate,
	cacheMode string,
	cutoff time.Time,
) (matchAggregates, bool) {
	// Try cache read if enabled
	if cacheMode == "read" || cacheMode == "readwrite" {
		if rec, err := getMatchPredictionAggregatesFunc(ctx, m.MatchID); err == nil {
			predAgg := matchAggregates{
				Runs:           rec.PredictedTotalRuns.Float64,
				WinnerTeamCode: rec.PredictedWinnerCode.String,
			}
			return predAgg, true
		}
	}

	// Fallback to ML prediction
	p, modelVersion, err := mlBacktestPredictMatchAggregatesFunc(ctx, cutoff, [2]string{m.Team1, m.Team2})
	if err != nil {
		return matchAggregates{}, false
	}

	// Optionally write to cache
	if cacheMode == "readwrite" {
		if err := upsertMatchPredictionAggregatesFunc(ctx, db.MatchPredictionAggregates{
			MatchID:             m.MatchID,
			Format:              m.Format,
			Team1Code:           m.Team1,
			Team2Code:           m.Team2,
			PredictedWinnerCode: sqlNullString(p.WinnerTeamCode),
			PredictedTotalRuns:  sqlNullFloat64(p.Runs),
			ModelVersion:        sqlNullString(modelVersion),
			CutoffAt:            cutoff,
		}); err != nil {
			// Failing to cache is not critical for the request, but should be monitored
			slog.Warn(
				"failed to upsert match prediction aggregates cache",
				slog.Any("err", err),
				slog.Int64("match_id", m.MatchID),
				slog.String("format", m.Format),
				slog.String("team1", m.Team1),
				slog.String("team2", m.Team2),
				slog.Time("cutoff_at", cutoff),
			)
		}
	}

	return p, true
}

// helpers moved to helpers.go for reuse across the server package

// listAccuracyTrendCandidates wraps the played matches listing with clear intent.
// It is a thin layer over the existing seam to keep handlers small and descriptive.
func listAccuracyTrendCandidates(
	ctx context.Context,
	format string,
	team1 string,
	team2 string,
	start time.Time,
	end time.Time,
	order string,
	limit int,
) ([]backtestCandidate, error) {
	return listPlayedMatchesByFilters(ctx, format, team1, team2, start, end, order, limit)
}

// computeAccuracyTrendForCandidates computes metrics for each candidate using a
// batch ML prediction when possible (1 HTTP call instead of N), falling back to
// concurrent per-candidate calls if the batch endpoint is unavailable.
// When useUnifiedModel is true, player predictions use the unified (legacy) model instead of format-specific.
func computeAccuracyTrendForCandidates(
	ctx context.Context,
	candidates []backtestCandidate,
	includePlayer bool,
	includeTeam bool,
	cacheMode string,
	useUnifiedModel bool,
) ([]accuracyTrendItem, map[string]float64, []map[string]float64) {
	results := make([]accuracyTrendItem, len(candidates))

	if includePlayer {
		computePlayerMetricsBatch(ctx, candidates, results, useUnifiedModel)
	}

	if includeTeam {
		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(accuracyTrendConcurrency)
		for i, m := range candidates {
			i, m := i, m
			g.Go(func() error {
				cutoff := resolveCandidateCutoff(gCtx, m)
				if cutoff.IsZero() || m.Team1 == "" || m.Team2 == "" {
					return nil
				}
				if predAgg, ok := getPredictedAggregates(gCtx, m, cacheMode, cutoff); ok {
					if actAgg, errA := getBacktestMatchAggregatesActualsFunc(gCtx, m.MatchID); errA == nil {
						if results[i].Metrics == nil {
							results[i].Metrics = map[string]float64{}
						}
						if actAgg.Runs != 0 || predAgg.Runs != 0 {
							results[i].Metrics["team_runs_mae"] = math.Abs(predAgg.Runs - actAgg.Runs)
						}
						if v := winnerAccuracy(predAgg.WinnerTeamCode, actAgg.WinnerTeamCode); v > 0 ||
							(actAgg.WinnerTeamCode != "" && predAgg.WinnerTeamCode != "") {
							results[i].Metrics["team_winner_accuracy"] = v
						}
					}
				}
				return nil
			})
		}
		_ = g.Wait()
	}

	for i, m := range candidates {
		results[i].MatchID = m.MatchID
		results[i].MatchDate = m.MatchDate
		results[i].Format = m.Format
		results[i].Team1 = m.Team1
		results[i].Team2 = m.Team2
		if results[i].Metrics == nil {
			results[i].Metrics = map[string]float64{}
		}
	}

	items := make([]backtest.AccuracyTrendItem, len(results))
	for i := range results {
		items[i] = backtest.AccuracyTrendItem{
			MatchID:   results[i].MatchID,
			MatchDate: results[i].MatchDate,
			Format:    results[i].Format,
			Team1:     results[i].Team1,
			Team2:     results[i].Team2,
			Metrics:   results[i].Metrics,
		}
	}
	summary, progressive := backtest.ComputeSummaryAndProgressive(items)
	return results, summary, progressive
}

// computePlayerMetricsBatch resolves squads and actuals for all candidates in
// parallel, then attempts a single batch ML prediction call. Falls back to
// concurrent per-candidate calls if the batch endpoint is unavailable.
func computePlayerMetricsBatch(
	ctx context.Context,
	candidates []backtestCandidate,
	results []accuracyTrendItem,
	useUnifiedModel bool,
) {
	preps := make([]accuracyTrendCandidatePrep, len(candidates))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(accuracyTrendConcurrency)
	for i, m := range candidates {
		i, m := i, m
		g.Go(func() error {
			cutoff := resolveCandidateCutoff(gCtx, m)
			if cutoff.IsZero() {
				return nil
			}
			squad, err := getBacktestSquadPlayerIDsFunc(gCtx, m.MatchID, cutoff, m.Format)
			if err != nil || len(squad) == 0 {
				return nil
			}
			acts, err := getBacktestPlayerActualsForMatchFunc(gCtx, m.MatchID)
			if err != nil {
				return nil
			}
			fmt := m.Format
			if useUnifiedModel {
				fmt = ""
			}
			preps[i] = accuracyTrendCandidatePrep{cutoff: cutoff, squad: squad, actuals: acts, format: fmt}
			return nil
		})
	}
	_ = g.Wait()

	batchInputs := make([]BatchPredictPlayersInput, 0, len(candidates))
	batchIndexMap := make([]int, 0, len(candidates))
	for i, p := range preps {
		if len(p.squad) == 0 {
			continue
		}
		batchInputs = append(batchInputs, BatchPredictPlayersInput{
			Cutoff:    p.cutoff,
			Format:    p.format,
			PlayerIDs: p.squad,
		})
		batchIndexMap = append(batchIndexMap, i)
	}

	if len(batchInputs) == 0 {
		return
	}

	batchResults, err := mlBacktestPredictBatchFunc(ctx, batchInputs)
	if err != nil {
		slog.Warn("batch predict unavailable, falling back to per-candidate calls", slog.Any("err", err))
		computePlayerMetricsFallback(ctx, candidates, preps, results)
		return
	}

	for j, preds := range batchResults {
		idx := batchIndexMap[j]
		mae := computeMAEFromPreds(preps[idx].squad, preds, preps[idx].actuals)
		if mae >= 0 {
			if results[idx].Metrics == nil {
				results[idx].Metrics = map[string]float64{}
			}
			results[idx].Metrics["player_runs_mae"] = mae
		}
	}
}

// computePlayerMetricsFallback uses parallel per-candidate ML calls when batch is unavailable.
func computePlayerMetricsFallback(
	ctx context.Context,
	candidates []backtestCandidate,
	preps []accuracyTrendCandidatePrep,
	results []accuracyTrendItem,
) {
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(accuracyTrendConcurrency)
	for i := range candidates {
		i := i
		p := preps[i]
		if len(p.squad) == 0 {
			continue
		}
		g.Go(func() error {
			preds, err := mlBacktestPredictFunc(gCtx, p.cutoff, p.format, p.squad, nil, false, nil)
			if err != nil {
				return nil
			}
			mae := computeMAEFromPreds(p.squad, preds, p.actuals)
			if mae >= 0 {
				if results[i].Metrics == nil {
					results[i].Metrics = map[string]float64{}
				}
				results[i].Metrics["player_runs_mae"] = mae
			}
			return nil
		})
	}
	_ = g.Wait()
}

func computeMAEFromPreds(squad []int64, preds map[int64]playerPredictions, acts map[int64]playerActuals) float64 {
	var totalAbs, cnt float64
	for _, pid := range squad {
		pPred, okp := preds[pid]
		pAct, oka := acts[pid]
		if !okp || !oka {
			continue
		}
		totalAbs += math.Abs(pPred.Runs - pAct.Runs)
		cnt++
	}
	if cnt > 0 {
		return totalAbs / cnt
	}
	return -1
}
