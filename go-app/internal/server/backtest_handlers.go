package server

import (
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// backtestMatchHandler handles GET /api/backtest/match
// Modes:
// - select (default when match_id is absent): returns candidate played matches for given filters
// - evaluate (when match_id present)
func (a *App) backtestMatchHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := strings.TrimSpace(q.Get("format"))
	team1 := strings.TrimSpace(q.Get("team1"))
	team2 := strings.TrimSpace(q.Get("team2"))
	mode := strings.TrimSpace(q.Get("mode"))
	matchID := strings.TrimSpace(q.Get("match_id"))

	if format == "" || team1 == "" || team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}

	// Default to select mode if no match_id
	if mode == "" {
		if matchID == "" {
			mode = "select"
		} else {
			mode = "evaluate"
		}
	}

	if mode == "select" {
		rows, err := listPlayedByFmtTeams(r.Context(), format, team1, team2)
		if err != nil {
			respondErr(w, err)
			return
		}
		cands := make([]backtestCandidate, 0, len(rows))
		for _, row := range rows {
			cands = append(cands, backtestCandidate{
				MatchID:        row.MatchID,
				StableID:       nullString(row.StableID),
				Date:           row.Date.Format("2006-01-02T15:04:05Z07:00"),
				Venue:          nullString(row.Venue),
				Season:         nullString(row.Season),
				Format:         nullString(row.FormatCode),
				Team1:          row.Team1,
				Team2:          row.Team2,
				WinnerTeamCode: nullString(row.WinnerTeam),
			})
		}
		resp := backtestSelectResponse{
			Filters: map[string]any{
				"format": format,
				"team1":  team1,
				"team2":  team2,
			},
			Candidates: cands,
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Evaluate mode
	if matchID == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "match_id is required for evaluate mode"},
		)
		return
	}
	mid, err := strconv.ParseInt(matchID, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid match_id"})
		return
	}

	cutoff, err := getBacktestMatchDateFunc(r.Context(), mid)
	if err != nil {
		respondErr(w, err)
		return
	}
	squad, err := getBacktestSquadPlayerIDsFunc(r.Context(), mid, cutoff, format)
	if err != nil {
		respondErr(w, err)
		return
	}
	// Features currently unused in baseline; kept for future extension
	_, _ = getBacktestFeaturesAtCutoffFunc(r.Context(), cutoff, squad)

	preds, err := mlBacktestPredictFunc(r.Context(), cutoff, squad)
	if err != nil {
		respondErr(w, err)
		return
	}
	actuals, err := getBacktestPlayerActualsForMatchFunc(r.Context(), mid)
	if err != nil {
		respondErr(w, err)
		return
	}

	// Build response
	resp := backtestEvaluateResponse{
		Filters: map[string]any{
			"format":   format,
			"team1":    team1,
			"team2":    team2,
			"match_id": mid,
		},
	}
	// Match info
	resp.Match.MatchID = mid
	resp.Match.Date = cutoff.Format(time.RFC3339)
	// Players
	for _, pid := range squad {
		var pRes struct {
			PlayerID  int64              `json:"player_id"`
			Predicted map[string]float64 `json:"predicted"`
			Actual    map[string]float64 `json:"actual"`
			Errors    map[string]float64 `json:"errors"`
		}
		pRes.PlayerID = pid
		if pp, ok := preds[pid]; ok {
			pRes.Predicted = map[string]float64{
				"runs":     pp.Runs,
				"wickets":  pp.Wickets,
				"economy":  pp.Economy,
				"catches":  pp.Catches,
				"run_outs": pp.RunOuts,
			}
		}
		if aa, ok := actuals[pid]; ok {
			pRes.Actual = map[string]float64{
				"runs":     aa.Runs,
				"wickets":  aa.Wickets,
				"economy":  aa.Economy,
				"catches":  aa.Catches,
				"run_outs": aa.RunOuts,
			}
		}
		// errors
		pRes.Errors = map[string]float64{}
		if pRes.Predicted != nil && pRes.Actual != nil {
			pRes.Errors["runs_mae"] = math.Abs(pRes.Predicted["runs"] - pRes.Actual["runs"])
			pRes.Errors["wickets_mae"] = math.Abs(pRes.Predicted["wickets"] - pRes.Actual["wickets"])
			pRes.Errors["economy_mae"] = math.Abs(pRes.Predicted["economy"] - pRes.Actual["economy"])
			pRes.Errors["catches_mae"] = math.Abs(pRes.Predicted["catches"] - pRes.Actual["catches"])
			pRes.Errors["run_outs_mae"] = math.Abs(pRes.Predicted["run_outs"] - pRes.Actual["run_outs"])
		}
		resp.Players = append(resp.Players, pRes)
	}

	// Metrics (player-level averages)
	resp.Metrics = map[string]float64{}
	var (
		totalAbsErrRuns, countRuns       float64
		totalSqErrRuns                   float64
		runsActuals                      []float64
		totalAbsErrWickets, countWickets float64
		totalAbsErrEcon, countEcon       float64
		totalAbsErrCatches, countCatches float64
		totalAbsErrRunOuts, countRunOuts float64
	)
	// Recompute metrics from preds/actuals to also accumulate squared errors
	for _, pid := range squad {
		pp, okP := preds[pid]
		aa, okA := actuals[pid]
		if okP && okA {
			diff := pp.Runs - aa.Runs
			totalAbsErrRuns += math.Abs(diff)
			totalSqErrRuns += diff * diff
			countRuns++
			runsActuals = append(runsActuals, aa.Runs)
			totalAbsErrWickets += math.Abs(pp.Wickets - aa.Wickets)
			countWickets++
			totalAbsErrEcon += math.Abs(pp.Economy - aa.Economy)
			countEcon++
			totalAbsErrCatches += math.Abs(pp.Catches - aa.Catches)
			countCatches++
			totalAbsErrRunOuts += math.Abs(pp.RunOuts - aa.RunOuts)
			countRunOuts++
		}
	}
	if countRuns > 0 {
		resp.Metrics["player_runs_mae"] = totalAbsErrRuns / countRuns
		// RMSE for runs
		resp.Metrics["player_runs_rmse"] = math.Sqrt(totalSqErrRuns / countRuns)
		// R² computation
		var meanA float64
		for _, v := range runsActuals {
			meanA += v
		}
		meanA /= float64(len(runsActuals))
		var ssTot float64
		for _, v := range runsActuals {
			d := v - meanA
			ssTot += d * d
		}
		if ssTot > 0 {
			resp.Metrics["player_runs_r2"] = 1.0 - (totalSqErrRuns / ssTot)
		} else {
			resp.Metrics["player_runs_r2"] = 0.0
		}
	}
	if countWickets > 0 {
		resp.Metrics["player_wickets_mae"] = totalAbsErrWickets / countWickets
	}
	if countEcon > 0 {
		resp.Metrics["player_economy_mae"] = totalAbsErrEcon / countEcon
	}
	if countCatches > 0 {
		resp.Metrics["player_catches_mae"] = totalAbsErrCatches / countCatches
	}
	if countRunOuts > 0 {
		resp.Metrics["player_run_outs_mae"] = totalAbsErrRunOuts / countRunOuts
	}

	// Match-level aggregates (optional if seams available)
	if predAgg, _, err1 := mlBacktestPredictMatchAggregatesFunc(r.Context(), cutoff, [2]string{team1, team2}); err1 == nil {
		if actAgg, err2 := getBacktestMatchAggregatesActualsFunc(r.Context(), mid); err2 == nil {
			// Populate response section
			resp.MatchAggregates.Predicted = map[string]any{
				"runs":             predAgg.Runs,
				"wickets":          predAgg.Wickets,
				"extras":           predAgg.Extras,
				"winner_team_code": predAgg.WinnerTeamCode,
			}
			resp.MatchAggregates.Actual = map[string]any{
				"runs":             actAgg.Runs,
				"wickets":          actAgg.Wickets,
				"extras":           actAgg.Extras,
				"winner_team_code": actAgg.WinnerTeamCode,
			}
			// Errors (MAE for scalar aggregates)
			resp.MatchAggregates.Errors = map[string]float64{}
			resp.MatchAggregates.Errors["runs_mae"] = math.Abs(predAgg.Runs - actAgg.Runs)
			resp.MatchAggregates.Errors["wickets_mae"] = math.Abs(predAgg.Wickets - actAgg.Wickets)
			resp.MatchAggregates.Errors["extras_mae"] = math.Abs(predAgg.Extras - actAgg.Extras)
			// Summary metrics mirror errors for single match
			resp.Metrics["match_runs_mae"] = resp.MatchAggregates.Errors["runs_mae"]
			resp.Metrics["match_wickets_mae"] = resp.MatchAggregates.Errors["wickets_mae"]
			resp.Metrics["match_extras_mae"] = resp.MatchAggregates.Errors["extras_mae"]
			if predAgg.WinnerTeamCode != "" && actAgg.WinnerTeamCode != "" {
				if strings.EqualFold(predAgg.WinnerTeamCode, actAgg.WinnerTeamCode) {
					resp.Metrics["winner_accuracy"] = 1
				} else {
					resp.Metrics["winner_accuracy"] = 0
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// backtestAccuracyTrendHandler handles GET /api/backtest/accuracy-trend
// Optional filters: format, start_date, end_date, team1, team2, order, limit
func (a *App) backtestAccuracyTrendHandler(w http.ResponseWriter, r *http.Request) {
	params, perr := parseBacktestAccuracyTrendParams(r)
	if perr != nil {
		switch perr {
		case errInvalidStartDate:
			writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid start_date"})
		case errInvalidEndDate:
			writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid end_date"})
		case errEndBeforeStart:
			writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "end_date before start_date"})
		default:
			respondErr(w, perr)
		}
		return
	}

	// List candidates via seam (tests will stub this)
	cands, err := listPlayedMatchesByFilters(
		r.Context(),
		params.Format, params.Team1, params.Team2,
		params.Start, params.End, params.Order, params.Limit,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		respondErr(w, err)
		return
	}
	if len(cands) == 0 {
		resp := accuracyTrendResponse{
			Filters: map[string]any{
				"format": params.Format, "team1": params.Team1, "team2": params.Team2,
				"start_date": params.RawStart, "end_date": params.RawEnd,
				"order": params.Order, "limit": params.Limit,
			},
			Count:       0,
			Results:     []accuracyTrendItem{},
			Summary:     map[string]float64{"n": 0},
			Progressive: []map[string]float64{},
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Compute per-match metrics
	results := make([]accuracyTrendItem, 0, len(cands))
	for _, m := range cands {
		metrics := computeAccuracyTrendMetrics(r.Context(), m, params.Cache, params.IncludePlayer, params.IncludeTeam)
		results = append(results, accuracyTrendItem{
			MatchID: m.MatchID,
			Date:    m.Date,
			Format:  m.Format,
			Team1:   m.Team1,
			Team2:   m.Team2,
			Metrics: metrics,
		})
	}

	// Summary and progressive
	summary, progressive := computeAccuracyTrendSummaryAndProgressive(results)

	resp := accuracyTrendResponse{
		Filters: map[string]any{
			"format": params.Format, "team1": params.Team1, "team2": params.Team2,
			"start_date": params.RawStart, "end_date": params.RawEnd,
			"order": params.Order, "limit": params.Limit,
		},
		Count:       len(results),
		Results:     results,
		Summary:     summary,
		Progressive: progressive,
	}
	writeJSON(w, http.StatusOK, resp)
}
