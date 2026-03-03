// Package backtest provides backtest domain logic (accuracy trend aggregation, etc.)
// so HTTP handlers can stay thin and delegate to this package.
package backtest

// EmptySummaryAndProgressive returns the canonical empty summary and progressive
// slice for the accuracy-trend API when there are no candidates.
func EmptySummaryAndProgressive() (summary map[string]float64, progressive []map[string]float64) {
	return map[string]float64{"n": 0}, nil
}

// AccuracyTrendItem is one match's metrics for the accuracy-trend API.
type AccuracyTrendItem struct {
	MatchID   int64
	MatchDate string
	Format    string
	Team1     string
	Team2     string
	Metrics   map[string]float64
}

// ComputeSummaryAndProgressive builds summary and progressive aggregates from
// a list of accuracy-trend items. For each metric, averages use the count of
// matches where that metric is present as the denominator.
func ComputeSummaryAndProgressive(items []AccuracyTrendItem) (summary map[string]float64, progressive []map[string]float64) {
	var (
		sumPlayerMAE, nPlayerMAE     float64
		sumTeamRunsMAE, nTeamRunsMAE  float64
		sumWinnerAcc, nWinnerAcc     float64
	)
	nMatches := float64(len(items))

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

	summary = map[string]float64{"n": nMatches}
	if nPlayerMAE > 0 {
		summary["player_runs_mae_avg"] = sumPlayerMAE / nPlayerMAE
	}
	if nTeamRunsMAE > 0 {
		summary["team_runs_mae_avg"] = sumTeamRunsMAE / nTeamRunsMAE
	}
	if nWinnerAcc > 0 {
		summary["team_winner_accuracy_avg"] = sumWinnerAcc / nWinnerAcc
	}

	progressive = make([]map[string]float64, 0, len(items))
	var (
		psPlayer, pnPlayer     float64
		psTeamRuns, pnTeamRuns float64
		psWinner, pnWinner     float64
	)
	for i, it := range items {
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
		row := map[string]float64{"n": float64(i + 1)}
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
