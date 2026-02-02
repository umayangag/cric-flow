package server

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// resolveCandidateCutoff determines the cutoff timestamp for a backtest candidate.
// Preference order:
// 1) Database match date via seam getBacktestMatchDateFunc
// 2) Fallback to parsing the candidate's Date (RFC3339)
// Returns zero time if both sources are unavailable/invalid.
func resolveCandidateCutoff(ctx context.Context, m backtestCandidate) time.Time {
	if cutoff, err := getBacktestMatchDateFunc(ctx, m.MatchID); err == nil && !cutoff.IsZero() {
		return cutoff
	}
	if t, err := time.Parse(time.RFC3339, m.Date); err == nil {
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

// computePlayerRunsMAE fetches player-level predictions and actuals and
// returns the Mean Absolute Error (runs) along with a success flag.
// It encapsulates the nested checks used previously to improve readability.
func computePlayerRunsMAE(
	ctx context.Context,
	m backtestCandidate,
	cutoff time.Time,
) (float64, bool) {
	squad, err := getBacktestSquadPlayerIDsFunc(ctx, m.MatchID, cutoff, m.Format)
	if err != nil || len(squad) == 0 {
		return 0, false
	}

	preds, err1 := mlBacktestPredictFunc(ctx, cutoff, squad)
	acts, err2 := getBacktestPlayerActualsForMatchFunc(ctx, m.MatchID)
	if err1 != nil || err2 != nil {
		return 0, false
	}

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
		return totalAbs / cnt, true
	}
	return 0, false
}

// computeAccuracyTrendMetrics calculates metrics for a single match candidate.
// It mirrors the previous inline logic to avoid behavior changes.
func computeAccuracyTrendMetrics(
	ctx context.Context,
	m backtestCandidate,
	cacheMode string,
	includePlayer, includeTeam bool,
) map[string]float64 {
	metrics := map[string]float64{}

	// Determine cutoff (match date)
	cutoff := resolveCandidateCutoff(ctx, m)
	if cutoff.IsZero() {
		return metrics
	}

	// Player-level MAE on runs (optional)
	if includePlayer {
		if mae, ok := computePlayerRunsMAE(ctx, m, cutoff); ok {
			metrics["player_runs_mae"] = mae
		}
	}

	// Match/team aggregates with optional cache (optional)
	if includeTeam && m.Team1 != "" && m.Team2 != "" {
		if predAgg, ok := getPredictedAggregates(ctx, m, cacheMode, cutoff); ok {
			if actAgg, errA := getBacktestMatchAggregatesActualsFunc(ctx, m.MatchID); errA == nil {
				if actAgg.Runs != 0 || predAgg.Runs != 0 {
					metrics["team_runs_mae"] = math.Abs(predAgg.Runs - actAgg.Runs)
				}
				if v := winnerAccuracy(predAgg.WinnerTeamCode, actAgg.WinnerTeamCode); v > 0 ||
					(actAgg.WinnerTeamCode != "" && predAgg.WinnerTeamCode != "") {
					// Only set when both are non-empty; winnerAccuracy returns 0.0 otherwise
					metrics["team_winner_accuracy"] = v
				}
			}
		}
	}

	return metrics
}

// computeAccuracyTrendSummaryAndProgressive builds the summary and progressive rows
// from the list of result items. For each metric, averages are computed using the
// count of matches where the metric is actually present as the denominator.
func computeAccuracyTrendSummaryAndProgressive(items []accuracyTrendItem) (map[string]float64, []map[string]float64) {
	var (
		sumPlayerMAE, nPlayerMAE     float64
		sumTeamRunsMAE, nTeamRunsMAE float64
		sumWinnerAcc, nWinnerAcc     float64
	)
	nMatches := float64(len(items))

	// Summary accumulators
	for _, it := range items {
		if v, ok := it.Metrics["player_runs_mae"]; ok {
			sumPlayerMAE += v
			nPlayerMAE++
		}
		if v, ok := it.Metrics["team_runs_mae"]; ok {
			sumTeamRunsMAE += v
			nTeamRunsMAE++
		}
		if v, ok := it.Metrics["team_winner_accuracy"]; ok {
			sumWinnerAcc += v
			nWinnerAcc++
		}
	}

	summary := map[string]float64{"n": nMatches}
	if nPlayerMAE > 0 {
		summary["player_runs_mae_avg"] = sumPlayerMAE / nPlayerMAE
	}
	if nTeamRunsMAE > 0 {
		summary["team_runs_mae_avg"] = sumTeamRunsMAE / nTeamRunsMAE
	}
	if nWinnerAcc > 0 {
		summary["team_winner_accuracy_avg"] = sumWinnerAcc / nWinnerAcc
	}

	// Progressive calculations using per-metric present counts
	progressive := make([]map[string]float64, 0, len(items))
	var (
		psPlayer, pnPlayer     float64
		psTeamRuns, pnTeamRuns float64
		psWinner, pnWinner     float64
	)
	for i, it := range items {
		n := float64(i + 1)
		if v, ok := it.Metrics["player_runs_mae"]; ok {
			psPlayer += v
			pnPlayer++
		}
		if v, ok := it.Metrics["team_runs_mae"]; ok {
			psTeamRuns += v
			pnTeamRuns++
		}
		if v, ok := it.Metrics["team_winner_accuracy"]; ok {
			psWinner += v
			pnWinner++
		}
		row := map[string]float64{"n": n}
		if pnPlayer > 0 {
			row["player_runs_mae_avg"] = psPlayer / pnPlayer
		}
		if pnTeamRuns > 0 {
			row["team_runs_mae_avg"] = psTeamRuns / pnTeamRuns
		}
		if pnWinner > 0 {
			row["team_winner_accuracy_avg"] = psWinner / pnWinner
		}
		progressive = append(progressive, row)
	}

	return summary, progressive
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

// computeAccuracyTrendForCandidates computes metrics for each candidate and returns
// the items along with the summary and progressive aggregates.
func computeAccuracyTrendForCandidates(
	ctx context.Context,
	candidates []backtestCandidate,
	includePlayer bool,
	includeTeam bool,
	cacheMode string,
) ([]accuracyTrendItem, map[string]float64, []map[string]float64) {
	results := make([]accuracyTrendItem, 0, len(candidates))
	for _, m := range candidates {
		metrics := computeAccuracyTrendMetrics(ctx, m, cacheMode, includePlayer, includeTeam)
		results = append(results, accuracyTrendItem{
			MatchID: m.MatchID,
			Date:    m.Date,
			Format:  m.Format,
			Team1:   m.Team1,
			Team2:   m.Team2,
			Metrics: metrics,
		})
	}
	summary, progressive := computeAccuracyTrendSummaryAndProgressive(results)
	return results, summary, progressive
}
