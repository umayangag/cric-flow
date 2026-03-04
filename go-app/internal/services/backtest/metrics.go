package backtest

import (
	"math"
	"strings"
)

// ComputeR2 returns the coefficient of determination given total squared error and actual values.
func ComputeR2(totalSquaredError float64, actuals []float64) float64 {
	if len(actuals) == 0 {
		return 0
	}
	var mean float64
	for _, v := range actuals {
		mean += v
	}
	mean /= float64(len(actuals))
	var ssTot float64
	for _, v := range actuals {
		d := v - mean
		ssTot += d * d
	}
	if ssTot <= 0 {
		return 0
	}
	return 1.0 - (totalSquaredError / ssTot)
}

// WinnerAccuracy returns 1.0 when winner codes match (case-insensitive), else 0.0.
// Returns 0.0 if either code is empty.
func WinnerAccuracy(predWinner, actualWinner string) float64 {
	if predWinner == "" || actualWinner == "" {
		return 0
	}
	if strings.EqualFold(predWinner, actualWinner) {
		return 1
	}
	return 0
}

// ChooseBacktestMode decides the mode based on explicit input and presence of match_id.
// Defaults to "select" when match_id is empty, otherwise "evaluate".
func ChooseBacktestMode(modeInput, matchID string) string {
	m := strings.TrimSpace(modeInput)
	if m == "" {
		if strings.TrimSpace(matchID) == "" {
			return "select"
		}
		return "evaluate"
	}
	return m
}

// ComputePlayerResultsAndMetrics computes per-player error metrics and aggregate metrics
// from predicted and actual values for a squad of players.
func ComputePlayerResultsAndMetrics(
	squad []int64,
	preds map[int64]PlayerPredictions,
	actuals map[int64]PlayerActuals,
) ([]PlayerResult, map[string]float64) {
	players := make([]PlayerResult, 0, len(squad))
	metrics := map[string]float64{}
	var (
		totalAbsErrRuns, countRuns       float64
		totalSqErrRuns                   float64
		runsActuals                      []float64
		totalAbsErrWickets, countWickets float64
		totalAbsErrEcon, countEcon       float64
		totalAbsErrCatches, countCatches float64
		totalAbsErrRunOuts, countRunOuts float64
	)
	for _, pid := range squad {
		pp, okP := preds[pid]
		aa, okA := actuals[pid]
		if !okP || !okA {
			continue
		}
		var pRes PlayerResult
		pRes.PlayerID = pid
		pRes.Predicted = map[string]float64{
			"runs":     pp.Runs,
			"wickets":  pp.Wickets,
			"economy":  pp.Economy,
			"catches":  pp.Catches,
			"run_outs": pp.RunOuts,
		}
		pRes.Actual = map[string]float64{
			"runs":     aa.Runs,
			"wickets":  aa.Wickets,
			"economy":  aa.Economy,
			"catches":  aa.Catches,
			"run_outs": aa.RunOuts,
		}
		diffRuns := pp.Runs - aa.Runs
		pRes.Errors = map[string]float64{
			"runs_mae":     math.Abs(diffRuns),
			"wickets_mae":  math.Abs(pp.Wickets - aa.Wickets),
			"economy_mae":  math.Abs(pp.Economy - aa.Economy),
			"catches_mae":  math.Abs(pp.Catches - aa.Catches),
			"run_outs_mae": math.Abs(pp.RunOuts - aa.RunOuts),
		}
		players = append(players, pRes)

		totalAbsErrRuns += pRes.Errors["runs_mae"]
		totalSqErrRuns += diffRuns * diffRuns
		countRuns++
		runsActuals = append(runsActuals, aa.Runs)
		totalAbsErrWickets += pRes.Errors["wickets_mae"]
		countWickets++
		totalAbsErrEcon += pRes.Errors["economy_mae"]
		countEcon++
		totalAbsErrCatches += pRes.Errors["catches_mae"]
		countCatches++
		totalAbsErrRunOuts += pRes.Errors["run_outs_mae"]
		countRunOuts++
	}
	if countRuns > 0 {
		metrics["player_runs_mae"] = totalAbsErrRuns / countRuns
		metrics["player_runs_rmse"] = math.Sqrt(totalSqErrRuns / countRuns)
		metrics["player_runs_r2"] = ComputeR2(totalSqErrRuns, runsActuals)
	}
	if countWickets > 0 {
		metrics["player_wickets_mae"] = totalAbsErrWickets / countWickets
	}
	if countEcon > 0 {
		metrics["player_economy_mae"] = totalAbsErrEcon / countEcon
	}
	if countCatches > 0 {
		metrics["player_catches_mae"] = totalAbsErrCatches / countCatches
	}
	if countRunOuts > 0 {
		metrics["player_run_outs_mae"] = totalAbsErrRunOuts / countRunOuts
	}
	return players, metrics
}

// FormatFromFilters extracts the "format" string from a filters map, trimmed and uppercased.
func FormatFromFilters(filters map[string]any) string {
	if filters == nil {
		return ""
	}
	if v, ok := filters["format"]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(strings.ToUpper(s))
		}
	}
	return ""
}
