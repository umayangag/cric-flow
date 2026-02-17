package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	exq "github.com/umayangag/cric-info-scrapers/go-app/internal/db/exportqueries"
)

// --- Helpers extracted for readability (no behavior change) ---

// chooseBacktestMode decides the mode based on explicit input and presence of match_id.
// If mode is empty, defaults to "select" when match_id is empty, otherwise "evaluate".
func chooseBacktestMode(modeInput, matchID string) string {
	m := strings.TrimSpace(modeInput)
	if m == "" {
		if strings.TrimSpace(matchID) == "" {
			return "select"
		}
		return "evaluate"
	}
	return m
}

// computeR2 returns the coefficient of determination given total squared error and actual values.
func computeR2(totalSquaredError float64, actuals []float64) float64 {
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

// winnerAccuracy computes 1.0 when winner codes match (case-insensitive), else 0.0; returns 0.0 if any is empty.
func winnerAccuracy(predWinner, actualWinner string) float64 {
	if predWinner == "" || actualWinner == "" {
		return 0
	}
	if strings.EqualFold(predWinner, actualWinner) {
		return 1
	}
	return 0
}

// buildPredictedScorecard builds a scorecard from the actual layout with ML-predicted stats per player.
// Predictions use only data before the match date. Batting rows get predicted runs; bowling rows get predicted wickets, economy, and derived runs.
func buildPredictedScorecard(actual *db.MatchScorecard, preds map[int64]playerPredictions) *db.MatchScorecard {
	if actual == nil {
		return nil
	}
	out := &db.MatchScorecard{
		MatchID:   actual.MatchID,
		MatchDate: actual.MatchDate,
		Venue:     actual.Venue,
		Innings:   make([]db.ScorecardInning, 0, len(actual.Innings)),
	}
	for _, in := range actual.Innings {
		inn := db.ScorecardInning{
			InningNumber:    in.InningNumber,
			BattingTeamName: in.BattingTeamName,
			BowlingTeamName: in.BowlingTeamName,
			Extras:          in.Extras,
			TargetRuns:      in.TargetRuns,
			Batting:         make([]db.ScorecardBatting, 0, len(in.Batting)),
			Bowling:         make([]db.ScorecardBowling, 0, len(in.Bowling)),
		}
		var predRunsSum int
		for _, b := range in.Batting {
			p := preds[b.PlayerID]
			r := int(math.Round(p.Runs))
			predRunsSum += r
			inn.Batting = append(inn.Batting, db.ScorecardBatting{
				PlayerID:   b.PlayerID,
				PlayerName: b.PlayerName,
				Runs:       intPtr(r),
				Balls:      nil,
				Fours:      nil,
				Sixes:      nil,
				StrikeRate: nil,
				HowOut:     nil,
			})
		}
		inn.RunsScored = predRunsSum
		var predWicketsSum int
		for _, w := range in.Bowling {
			p := preds[w.PlayerID]
			wkts := int(math.Round(p.Wickets))
			predWicketsSum += wkts
			ec := float32Ptr(float32(p.Economy))
			var predRuns *int
			if w.Balls != nil && *w.Balls > 0 {
				// Use balls (not overs) since overs decimal is balls (e.g. 3.5 overs = 23 balls)
				predRuns = intPtr(int(math.Round(float64(*w.Balls) * p.Economy / 6)))
			} else if w.Overs != nil && *w.Overs > 0 {
				predRuns = intPtr(int(math.Round(float64(*w.Overs) * p.Economy)))
			}
			inn.Bowling = append(inn.Bowling, db.ScorecardBowling{
				PlayerID:   w.PlayerID,
				PlayerName: w.PlayerName,
				Overs:      w.Overs,
				Maidens:    nil,
				Runs:       predRuns,
				Wickets:    intPtr(wkts),
				Economy:    ec,
				Wides:      nil,
				NoBalls:    nil,
				Balls:      w.Balls,
			})
		}
		inn.WicketsLost = predWicketsSum
		out.Innings = append(out.Innings, inn)
	}
	return out
}

func intPtr(n int) *int {
	v := n
	return &v
}

func float32Ptr(f float32) *float32 {
	v := f
	return &v
}

// computePlayerResultsAndMetrics walks through the given squad, pairing predictions with
// actuals to produce per-player results and summary metrics.
// Metrics computed:
// - player_runs_mae, player_runs_rmse, player_runs_r2
// - player_wickets_mae, player_economy_mae, player_catches_mae, player_run_outs_mae
// Behavior mirrors the inline logic previously in backtestMatchHandler.
func computePlayerResultsAndMetrics(
	squad []int64,
	preds map[int64]playerPredictions,
	actuals map[int64]playerActuals,
) ([]BacktestPlayerResult, map[string]float64) {
	players := make([]BacktestPlayerResult, 0, len(squad))
	metrics := map[string]float64{}

	var (
		totalAbsErrRuns, countRuns float64
		totalSqErrRuns             float64
		runsActuals                []float64

		totalAbsErrWickets, countWickets float64
		totalAbsErrEcon, countEcon       float64
		totalAbsErrCatches, countCatches float64
		totalAbsErrRunOuts, countRunOuts float64
	)

	for _, pid := range squad {
		pp, okP := preds[pid]
		aa, okA := actuals[pid]
		if !okP || !okA {
			// Skip players without both prediction and actuals
			continue
		}

		// Player result item
		var pRes BacktestPlayerResult
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

		// Aggregate for metrics
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
		metrics["player_runs_r2"] = computeR2(totalSqErrRuns, runsActuals)
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

// populateMatchAggregatesAndMetrics fills the response with match-level predicted/actual aggregates
// and associated metrics. Predicted aggregates are derived from player predictions (no baseline/RNG).
// Actuals come from DB. No behavior change if actuals seam fails.
func populateMatchAggregatesAndMetrics(
	ctx context.Context,
	resp *backtestEvaluateResponse,
	_ time.Time,
	team1 string,
	team2 string,
	matchID int64,
) {
	// Predicted: sum from player predictions; winner from team run totals.
	var predRuns, predWickets float64
	teamRuns := make(map[string]float64)
	playerTeams, err := db.GetMatchPlayerTeams(ctx, matchID)
	if err != nil {
		slog.WarnContext(ctx, "failed to get match player teams", slog.Int64("match_id", matchID), slog.Any("err", err))
		playerTeams = nil
	}
	if playerTeams == nil {
		playerTeams = make(map[int64]string)
	}
	for _, p := range resp.Players {
		r := 0.0
		if p.Predicted != nil {
			if v, ok := p.Predicted["runs"]; ok {
				r = v
			}
		}
		predRuns += r
		if t, ok := playerTeams[p.PlayerID]; ok {
			teamRuns[t] += r
		}
		w := 0.0
		if p.Predicted != nil {
			if v, ok := p.Predicted["wickets"]; ok {
				w = v
			}
		}
		predWickets += w
	}
	predWinner := ""
	if teamRuns[team1] > teamRuns[team2] {
		predWinner = team1
	} else if teamRuns[team2] > teamRuns[team1] {
		predWinner = team2
	}

	actAgg, err2 := getBacktestMatchAggregatesActualsFunc(ctx, matchID)
	if err2 != nil {
		return
	}
	// Predicted extras from historical average for this format/venue (no default constant)
	predExtras := 0.0
	if mfc, err := db.GetMatchFeatureContext(ctx, matchID); err != nil {
		slog.WarnContext(ctx, "failed to get match feature context for extras prediction", slog.Int64("match_id", matchID), slog.Any("err", err))
	} else {
		if avg, err := db.GetAverageExtrasForFormat(ctx, mfc.FormatID, mfc.VenueID); err != nil {
			slog.WarnContext(ctx, "failed to get average extras for format", slog.Int64("format_id", mfc.FormatID), slog.Any("err", err))
		} else {
			predExtras = avg
		}
	}
	resp.MatchAggregates.Predicted = map[string]any{
		"runs":             predRuns,
		"wickets":          predWickets,
		"extras":           predExtras,
		"winner_team_code": predWinner,
	}
	resp.MatchAggregates.Actual = map[string]any{
		"runs":             actAgg.Runs,
		"wickets":          actAgg.Wickets,
		"extras":           actAgg.Extras,
		"winner_team_code": actAgg.WinnerTeamCode,
	}
	resp.MatchAggregates.Errors = map[string]float64{}
	resp.MatchAggregates.Errors["runs_mae"] = math.Abs(predRuns - actAgg.Runs)
	resp.MatchAggregates.Errors["wickets_mae"] = math.Abs(predWickets - actAgg.Wickets)
	resp.MatchAggregates.Errors["extras_mae"] = math.Abs(predExtras - actAgg.Extras)
	if resp.Metrics == nil {
		resp.Metrics = map[string]float64{}
	}
	resp.Metrics["match_runs_mae"] = resp.MatchAggregates.Errors["runs_mae"]
	resp.Metrics["match_wickets_mae"] = resp.MatchAggregates.Errors["wickets_mae"]
	resp.Metrics["match_extras_mae"] = resp.MatchAggregates.Errors["extras_mae"]
	resp.Metrics["winner_accuracy"] = winnerAccuracy(predWinner, actAgg.WinnerTeamCode)
}

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
	useML := strings.TrimSpace(q.Get("use_ml"))
	cutoff := strings.TrimSpace(q.Get("cutoff"))

	if format == "" || team1 == "" || team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}

	// Default to select mode if no match_id
	mode = chooseBacktestMode(mode, matchID)

	if mode == "select" {
		a.handleBacktestSelect(r.Context(), w, format, team1, team2)
		return
	}

	// Evaluate mode
	a.handleBacktestEvaluate(r.Context(), w, format, team1, team2, matchID, useML, cutoff)
}

// handleBacktestSelect serves the select mode for the backtest endpoint.
// It lists candidate played matches given format and team filters.
func (a *App) handleBacktestSelect(ctx context.Context, w http.ResponseWriter, format, team1, team2 string) {
	rows, err := listPlayedByFmtTeams(ctx, format, team1, team2)
	if err != nil {
		respondErr(w, err)
		return
	}
	candidates := make([]backtestCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, backtestCandidate{
			MatchID:        row.MatchID,
			StableID:       nullString(row.StableID),
			MatchDate:      row.MatchDate.Format("2006-01-02T15:04:05Z07:00"),
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
		Candidates: candidates,
	}
	writeJSON(w, http.StatusOK, resp)
}

// BacktestProgressFunc is called after each step during evaluate (e.g. for SSE progress).
// step is a short id (e.g. "match_date", "squad", "features", "ml_predict", "actuals", "scorecard"); message is human-readable.
type BacktestProgressFunc func(step, message string)

// doEvaluateWork runs the default evaluate pipeline (no use_ml=1). Progress is called after each step when non-nil.
func doEvaluateWork(
	ctx context.Context,
	format, team1, team2, matchID string,
	progress BacktestProgressFunc,
) (*backtestEvaluateResponse, error) {
	mid, err := strconv.ParseInt(matchID, 10, 64)
	if err != nil {
		return nil, err
	}

	if progress != nil {
		progress("match_date", "Getting match date (cutoff for features)...")
	}
	cutoff, err := getBacktestMatchDateFunc(ctx, mid)
	if err != nil {
		return nil, err
	}

	if progress != nil {
		progress("squad", "Loading playing XI (squad) for this match...")
	}
	squad, err := getBacktestSquadPlayerIDsFunc(ctx, mid, cutoff, format)
	if err != nil {
		return nil, err
	}

	if progress != nil {
		progress("features", "Computing feature data at cutoff (no future data)...")
	}
	features, err := getBacktestFeaturesAtCutoffFunc(ctx, cutoff, squad, mid, format)
	if err != nil {
		return nil, err
	}

	if progress != nil {
		progress("ml_predict", "Calling ML model for player predictions (batting, bowling, fielding when loaded)...")
	}
	preds, err := mlBacktestPredictFunc(ctx, cutoff, format, squad, features)
	if err != nil {
		return nil, err
	}

	if progress != nil {
		progress("actuals", "Loading actual match stats from database...")
	}
	actuals, err := getBacktestPlayerActualsForMatchFunc(ctx, mid)
	if err != nil {
		return nil, err
	}

	if progress != nil {
		progress("metrics", "Computing player metrics and errors...")
	}
	resp := backtestEvaluateResponse{
		Filters: map[string]any{
			"format":   format,
			"team1":    team1,
			"team2":    team2,
			"match_id": mid,
		},
	}
	resp.Match.MatchID = mid
	resp.Match.MatchDate = cutoff.Format(time.RFC3339)
	players, metrics := computePlayerResultsAndMetrics(squad, preds, actuals)
	resp.Players = append(resp.Players, players...)
	resp.Metrics = metrics

	if progress != nil {
		progress("aggregates", "Fetching match-level aggregates...")
	}
	populateMatchAggregatesAndMetrics(ctx, &resp, cutoff, team1, team2, mid)

	if progress != nil {
		progress("scorecard", "Building predicted scorecard...")
	}
	if actualCard, err := db.GetMatchScorecard(ctx, mid); err == nil && actualCard != nil {
		resp.PredictedScorecard = buildPredictedScorecard(actualCard, preds)
	}

	if progress != nil {
		progress("done", "Evaluation complete.")
	}
	return &resp, nil
}

// handleBacktestEvaluate serves the evaluate mode for the backtest endpoint.
// It requires a valid matchID and computes per-player results and summary metrics.
func (a *App) handleBacktestEvaluate(
	ctx context.Context,
	w http.ResponseWriter,
	format, team1, team2, matchID string,
	useMLFlag string,
	cutoffStr string,
) {
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

	// Optional delegation to ML historical backtest endpoint if requested via query flag use_ml=1
	if strings.EqualFold(strings.TrimSpace(useMLFlag), "1") {
		if cutoffStr == "" {
			writeJSON(
				w,
				http.StatusBadRequest,
				apiError{Code: "INVALID_PARAM", Message: "cutoff (RFC3339) is required when use_ml=1"},
			)
			return
		}
		cutoff, perr := time.Parse(time.RFC3339, cutoffStr)
		if perr != nil {
			writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid cutoff timestamp"})
			return
		}
		res, herr := mlHistoricalBacktestFunc(ctx, cutoff, &mid, nil)
		if herr != nil {
			respondErr(w, herr)
			return
		}
		out := mapMLResponseToBacktestResponse(res, mid, format, team1, team2, cutoff)
		writeJSON(w, http.StatusOK, out)
		return
	}

	resp, err := doEvaluateWork(ctx, format, team1, team2, matchID, nil)
	if err != nil {
		respondErr(w, err)
		return
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

	// List candidates via helper (tests can stub seams used within)
	candidates, err := listAccuracyTrendCandidates(
		r.Context(),
		params.Format, params.Team1, params.Team2,
		params.Start, params.End, params.Order, params.Limit,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		respondErr(w, err)
		return
	}
	if len(candidates) == 0 {
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

	// Compute metrics and aggregates via helper
	results, summary, progressive := computeAccuracyTrendForCandidates(
		r.Context(), candidates, params.IncludePlayer, params.IncludeTeam, params.Cache,
	)

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

// mapMLResponseToBacktestResponse transforms the ML service response into the API response format.
func mapMLResponseToBacktestResponse(
	res HistoricalBacktestResult,
	matchID int64,
	format, team1, team2 string,
	cutoff time.Time,
) backtestEvaluateResponse {
	out := backtestEvaluateResponse{
		Filters: map[string]any{
			"format":        format,
			"team1":         team1,
			"team2":         team2,
			"match_id":      matchID,
			"delegated":     true,
			"model_version": res.ModelVersion,
		},
	}
	out.Match.MatchID = matchID
	out.Match.MatchDate = cutoff.Format(time.RFC3339)
	// Players
	players := make([]BacktestPlayerResult, 0, len(res.Players))
	for _, p := range res.Players {
		pr := BacktestPlayerResult{PlayerID: p.PlayerID}
		pr.Predicted = map[string]float64{"runs": p.Predicted.Runs}
		pr.Actual = map[string]float64{"runs": p.Actual.Runs}
		// optional fields
		if p.Predicted.Wickets != nil {
			pr.Predicted["wickets"] = *p.Predicted.Wickets
		}
		if p.Predicted.Economy != nil {
			pr.Predicted["economy"] = *p.Predicted.Economy
		}
		if p.Actual.Wickets != nil {
			pr.Actual["wickets"] = *p.Actual.Wickets
		}
		if p.Actual.Economy != nil {
			pr.Actual["economy"] = *p.Actual.Economy
		}
		errs := map[string]float64{"runs_mae": p.AbsErrorRuns}
		if p.AbsErrorWickets != nil {
			errs["wickets_mae"] = *p.AbsErrorWickets
		}
		pr.Errors = errs
		players = append(players, pr)
	}
	out.Players = players
	// Match aggregates
	out.MatchAggregates.Predicted = map[string]any{
		"runs":             res.Match.Predicted.Runs,
		"wickets":          res.Match.Predicted.Wickets,
		"extras":           res.Match.Predicted.Extras,
		"winner_team_code": res.Match.Predicted.WinnerTeamCode,
	}
	out.MatchAggregates.Actual = map[string]any{
		"runs":             res.Match.Actual.Runs,
		"wickets":          res.Match.Actual.Wickets,
		"extras":           res.Match.Actual.Extras,
		"winner_team_code": res.Match.Actual.WinnerTeamCode,
	}
	out.MatchAggregates.Errors = map[string]float64{
		"runs_mae":    math.Abs(res.Match.Predicted.Runs - res.Match.Actual.Runs),
		"wickets_mae": math.Abs(res.Match.Predicted.Wickets - res.Match.Actual.Wickets),
		"extras_mae":  math.Abs(res.Match.Predicted.Extras - res.Match.Actual.Extras),
	}
	// Summary metrics
	out.Metrics = map[string]float64{"player_runs_mae": res.Metrics.MAERuns}
	// include RMSE if available (typo safeguard)
	out.Metrics["player_runs_rmse"] = res.Metrics.RMSERuns
	if res.Metrics.MAEWickets != nil {
		out.Metrics["player_wickets_mae"] = *res.Metrics.MAEWickets
	}
	if res.Metrics.WinnerCorrect != nil {
		if *res.Metrics.WinnerCorrect {
			out.Metrics["winner_accuracy"] = 1
		} else {
			out.Metrics["winner_accuracy"] = 0
		}
	}
	return out
}

// backtestEvaluateStartHandler handles POST /api/backtest/evaluate-start (body or query: format, team1, team2, match_id).
// Starts evaluation in the background and returns { "job_id": "..." } so the client can poll evaluate-status.
func (a *App) backtestEvaluateStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST or GET required"})
		return
	}
	var format, team1, team2, matchID string
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
		var body struct {
			Format  string `json:"format"`
			Team1   string `json:"team1"`
			Team2   string `json:"team2"`
			MatchID int64  `json:"match_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(
				w,
				http.StatusBadRequest,
				apiError{Code: "INVALID_BODY", Message: "JSON body with format, team1, team2, match_id required"},
			)
			return
		}
		format = strings.TrimSpace(body.Format)
		team1 = strings.TrimSpace(body.Team1)
		team2 = strings.TrimSpace(body.Team2)
		if body.MatchID != 0 {
			matchID = strconv.FormatInt(body.MatchID, 10)
		}
	}
	if format == "" || team1 == "" || team2 == "" || matchID == "" {
		q := r.URL.Query()
		if format == "" {
			format = strings.TrimSpace(q.Get("format"))
		}
		if team1 == "" {
			team1 = strings.TrimSpace(q.Get("team1"))
		}
		if team2 == "" {
			team2 = strings.TrimSpace(q.Get("team2"))
		}
		if matchID == "" {
			matchID = strings.TrimSpace(q.Get("match_id"))
		}
	}
	if format == "" || team1 == "" || team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}
	if matchID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "match_id is required"})
		return
	}
	if _, err := strconv.ParseInt(matchID, 10, 64); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid match_id"})
		return
	}

	jobID, err := startEvaluateJob(r.Context(), format, team1, team2, matchID)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// backtestEvaluateStatusHandler handles GET /api/backtest/evaluate-status?job_id=...
// Returns current job state: status (running|done|error), steps, result (if done), error (if error).
func (a *App) backtestEvaluateStatusHandler(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "job_id is required"})
		return
	}
	snap, ok := getEvaluateJobStatus(jobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, apiError{Code: "NOT_FOUND", Message: "job not found (expired or invalid id)"})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// backtestEvaluateStreamHandler handles GET /api/backtest/evaluate-stream and streams progress via SSE, then the result.
// Query params: format, team1, team2, match_id (same as evaluate). use_ml=1 is not supported for streaming.
func (a *App) backtestEvaluateStreamHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := strings.TrimSpace(q.Get("format"))
	team1 := strings.TrimSpace(q.Get("team1"))
	team2 := strings.TrimSpace(q.Get("team2"))
	matchID := strings.TrimSpace(q.Get("match_id"))
	if format == "" || team1 == "" || team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}
	if matchID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "match_id is required"})
		return
	}
	if _, err := strconv.ParseInt(matchID, 10, 64); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid match_id"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	writeSSE := func(event, data string) bool {
		if _, err := w.Write([]byte("event: " + event + "\ndata: " + data + "\n\n")); err != nil {
			log.Printf("backtest evaluate-stream: write failed (client may have disconnected): %v", err)
			return false
		}
		flusher.Flush()
		return true
	}

	progress := func(step, message string) {
		payload := map[string]string{"step": step, "message": message}
		data, _ := json.Marshal(payload)
		writeSSE("progress", string(data))
	}

	resp, err := doEvaluateWork(r.Context(), format, team1, team2, matchID, progress)
	if err != nil {
		payload := map[string]string{"message": err.Error()}
		data, _ := json.Marshal(payload)
		if !writeSSE("error", string(data)) {
			return
		}
		return
	}
	resultData, err := json.Marshal(resp)
	if err != nil {
		payload := map[string]string{"message": "failed to encode result"}
		data, _ := json.Marshal(payload)
		if !writeSSE("error", string(data)) {
			return
		}
		return
	}
	// SSE event delimiter is \n\n; the data line must not contain literal newlines or the client will misparse.
	// json.Marshal produces compact (single-line) output and escapes newlines in string values as \\n, so no 0x0a appears.
	writeSSE("result", string(resultData))
}

// backtestScorecardHandler handles GET /api/backtest/scorecard?match_id=...
// It returns the match scorecard (innings, batting and bowling card) for the given match.
func (a *App) backtestScorecardHandler(w http.ResponseWriter, r *http.Request) {
	matchIDStr := strings.TrimSpace(r.URL.Query().Get("match_id"))
	if matchIDStr == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "match_id is required"})
		return
	}
	matchID, err := strconv.ParseInt(matchIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid match_id"})
		return
	}
	card, err := db.GetMatchScorecard(r.Context(), matchID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, apiError{Code: "NOT_FOUND", Message: "match not found"})
			return
		}
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

// trainingDataResponse is the JSON shape for GET /api/backtest/training-data (for ML service train-on-the-fly).
type trainingDataResponse struct {
	Batting  trainingDataPart `json:"batting"`
	Bowling  trainingDataPart `json:"bowling"`
	Fielding trainingDataPart `json:"fielding"`
	Extras   trainingDataPart `json:"extras"`
	Win      trainingDataPart `json:"win"`
}

type trainingDataPart struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// backtestTrainingDataHandler handles GET /api/backtest/training-data?cutoff=...&format=...
// cutoff (RFC3339) is required. format: use "all" (or omit) for all matches before cutoff; use a specific code (T20, ODI, etc.) to filter by that format.
func (a *App) backtestTrainingDataHandler(w http.ResponseWriter, r *http.Request) {
	cutoffStr := strings.TrimSpace(r.URL.Query().Get("cutoff"))
	if cutoffStr == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "cutoff is required (RFC3339)"})
		return
	}
	cutoff, err := time.Parse(time.RFC3339, cutoffStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "cutoff must be RFC3339"})
		return
	}
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	useAll := format == "" || strings.EqualFold(format, "all")
	var batRows, bowlRows, fieldRows, extrasRows, winRows [][]string
	if useAll {
		batRows, err = exq.BattingTrainingRows(r.Context(), cutoff)
		if err != nil {
			respondErr(w, err)
			return
		}
		bowlRows, err = exq.BowlingTrainingRows(r.Context(), cutoff)
		if err != nil {
			respondErr(w, err)
			return
		}
		fieldRows, err = exq.FieldingTrainingRows(r.Context(), cutoff)
		if err != nil {
			respondErr(w, err)
			return
		}
		extrasRows, err = exq.ExtrasTrainingRows(r.Context(), cutoff)
		if err != nil {
			respondErr(w, err)
			return
		}
		winRows, err = exq.WinTrainingRows(r.Context(), cutoff)
	} else {
		batRows, err = exq.BattingTrainingRowsWithFormat(r.Context(), format, cutoff)
		if err != nil {
			respondErr(w, err)
			return
		}
		bowlRows, err = exq.BowlingTrainingRowsWithFormat(r.Context(), format, cutoff)
		if err != nil {
			respondErr(w, err)
			return
		}
		fieldRows, err = exq.FieldingTrainingRowsWithFormat(r.Context(), format, cutoff)
		if err != nil {
			respondErr(w, err)
			return
		}
		extrasRows, err = exq.ExtrasTrainingRowsWithFormat(r.Context(), format, cutoff)
		if err != nil {
			respondErr(w, err)
			return
		}
		winRows, err = exq.WinTrainingRowsWithFormat(r.Context(), format, cutoff)
	}
	if err != nil {
		respondErr(w, err)
		return
	}
	part := func(rows [][]string) (headers []string, data [][]string) {
		if len(rows) > 0 {
			return rows[0], rows[1:]
		}
		return nil, nil
	}
	batH, batD := part(batRows)
	bowlH, bowlD := part(bowlRows)
	fieldH, fieldD := part(fieldRows)
	extrasH, extrasD := part(extrasRows)
	winH, winD := part(winRows)
	writeJSON(w, http.StatusOK, trainingDataResponse{
		Batting:  trainingDataPart{Headers: batH, Rows: batD},
		Bowling:  trainingDataPart{Headers: bowlH, Rows: bowlD},
		Fielding: trainingDataPart{Headers: fieldH, Rows: fieldD},
		Extras:   trainingDataPart{Headers: extrasH, Rows: extrasD},
		Win:      trainingDataPart{Headers: winH, Rows: winD},
	})
}

// matchesAfterResponse is the JSON shape for GET /api/backtest/matches (walk-forward).
type matchesAfterResponse struct {
	Matches []matchAfterItem `json:"matches"`
}

type matchAfterItem struct {
	MatchID   int64  `json:"match_id"`
	MatchDate string `json:"match_date"` // RFC3339
}

// backtestMatchesHandler handles GET /api/backtest/matches?after=...&format=...&limit=...
// Used by walk-forward: list match_id and match_date for matches strictly after the cutoff.
func (a *App) backtestMatchesHandler(w http.ResponseWriter, r *http.Request) {
	afterStr := strings.TrimSpace(r.URL.Query().Get("after"))
	if afterStr == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "after is required (RFC3339)"})
		return
	}
	after, err := time.Parse(time.RFC3339, afterStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "after must be RFC3339"})
		return
	}
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if format == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "format is required (e.g. T20, ODI)"})
		return
	}
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
			if limit > 500 {
				limit = 500
			}
		}
	}
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(r.Context(), format)
	if err != nil {
		respondErr(w, err)
		return
	}
	items, err := db.ListMatchIDsAfter(r.Context(), formatIDs, after, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	out := make([]matchAfterItem, 0, len(items))
	for _, it := range items {
		out = append(out, matchAfterItem{MatchID: it.MatchID, MatchDate: it.MatchDate.Format(time.RFC3339)})
	}
	writeJSON(w, http.StatusOK, matchesAfterResponse{Matches: out})
}

// backtestHoldoutDataHandler handles GET /api/backtest/holdout-data?cutoff=...&format=...&limit=...
// Returns training-data-shaped JSON for matches strictly after cutoff (features computed at cutoff) for walk-forward.
func (a *App) backtestHoldoutDataHandler(w http.ResponseWriter, r *http.Request) {
	cutoffStr := strings.TrimSpace(r.URL.Query().Get("cutoff"))
	if cutoffStr == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "cutoff is required (RFC3339)"})
		return
	}
	cutoff, err := time.Parse(time.RFC3339, cutoffStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "cutoff must be RFC3339"})
		return
	}
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if format == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "format is required (e.g. T20, ODI)"})
		return
	}
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
			if limit > 500 {
				limit = 500
			}
		}
	}
	batRows, err := exq.BattingHoldoutRows(r.Context(), format, cutoff, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	bowlRows, err := exq.BowlingHoldoutRows(r.Context(), format, cutoff, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	fieldRows, err := exq.FieldingHoldoutRows(r.Context(), format, cutoff, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	part := func(rows [][]string) (headers []string, data [][]string) {
		if len(rows) > 0 {
			return rows[0], rows[1:]
		}
		return nil, nil
	}
	batH, batD := part(batRows)
	bowlH, bowlD := part(bowlRows)
	fieldH, fieldD := part(fieldRows)
	writeJSON(w, http.StatusOK, trainingDataResponse{
		Batting:  trainingDataPart{Headers: batH, Rows: batD},
		Bowling:  trainingDataPart{Headers: bowlH, Rows: bowlD},
		Fielding: trainingDataPart{Headers: fieldH, Rows: fieldD},
		Extras:   trainingDataPart{},
		Win:      trainingDataPart{},
	})
}
