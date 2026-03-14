package server

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
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
	a.handleBacktestEvaluate(r.Context(), w, r, format, team1, team2, matchID, useML, cutoff)
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
// When useUnifiedModel is true, player predictions use the unified (legacy) model instead of format-specific.
// When useLatestModel is true, ML uses the latest available model (may include post-cutoff training data); otherwise strict temporal cutoff.
func doEvaluateWork(
	ctx context.Context,
	format, team1, team2, matchID string,
	useUnifiedModel, useLatestModel bool,
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
	formatForPrediction := format
	if useUnifiedModel {
		formatForPrediction = ""
	}
	preds, err := mlBacktestPredictFunc(ctx, cutoff, formatForPrediction, squad, features, useLatestModel, nil)
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
	modelMode := "strict_temporal"
	if useLatestModel {
		modelMode = "latest"
	}
	resp := backtestEvaluateResponse{
		Filters: map[string]any{
			"format":     format,
			"team1":      team1,
			"team2":      team2,
			"match_id":   mid,
			"model_mode": modelMode,
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
	populateMatchAggregatesAndMetrics(ctx, &resp, cutoff, team1, team2, mid, features)

	if progress != nil {
		progress("scorecard", "Building predicted scorecard...")
	}
	if actualCard, err := db.GetMatchScorecard(ctx, mid); err == nil && actualCard != nil {
		playerTeams, _ := db.GetMatchPlayerTeams(ctx, mid)
		if playerTeams == nil {
			playerTeams = map[int64]string{}
		}
		resp.PredictedScorecard = buildPredictedScorecard(actualCard, preds, playerTeams)
	}

	if progress != nil {
		progress("done", "Evaluation complete.")
	}
	return &resp, nil
}

// parseUseUnifiedModel reads use_unified_model or model=unified from the request (query or JSON body when applicable).
func parseUseUnifiedModel(r *http.Request, defaultVal bool) bool {
	q := r.URL.Query()
	if v := strings.TrimSpace(q.Get("use_unified_model")); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(q.Get("model")), "unified") {
		return true
	}
	return defaultVal
}

// parseUseLatestModel reads use_latest_model from the request (query or JSON body when applicable).
// When true, ML uses the latest available model (may include post-cutoff training data).
// Default is false (strict temporal: model trained only on data before match date).
func parseUseLatestModel(r *http.Request, defaultVal bool) bool {
	q := r.URL.Query()
	if v := strings.TrimSpace(q.Get("use_latest_model")); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	return defaultVal
}

// handleBacktestEvaluate serves the evaluate mode for the backtest endpoint.
// It requires a valid matchID and computes per-player results and summary metrics.
func (a *App) handleBacktestEvaluate(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
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

	useUnified := parseUseUnifiedModel(r, false)
	useLatest := parseUseLatestModel(r, false)
	resp, err := doEvaluateWork(ctx, format, team1, team2, matchID, useUnified, useLatest, nil)
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
		summary, progressive := backtest.EmptySummaryAndProgressive()
		resp := accuracyTrendResponse{
			Filters: map[string]any{
				"format": params.Format, "team1": params.Team1, "team2": params.Team2,
				"start_date": params.RawStart, "end_date": params.RawEnd,
				"order": params.Order, "limit": params.Limit,
				"use_unified_model": params.UseUnifiedModel,
			},
			Count:       0,
			Results:     []accuracyTrendItem{},
			Summary:     summary,
			Progressive: progressive,
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Compute metrics and aggregates via helper
	cfg := config.Load()
	concurrency := config.BacktestAccuracyTrendConcurrency(cfg)
	if concurrency <= 0 {
		concurrency = config.DefaultBacktestAccuracyTrendConcurrency
	}
	results, summary, progressive := computeAccuracyTrendForCandidates(
		r.Context(),
		candidates,
		params.IncludePlayer,
		params.IncludeTeam,
		params.Cache,
		params.UseUnifiedModel,
		concurrency,
	)

	resp := accuracyTrendResponse{
		Filters: map[string]any{
			"format": params.Format, "team1": params.Team1, "team2": params.Team2,
			"start_date": params.RawStart, "end_date": params.RawEnd,
			"order": params.Order, "limit": params.Limit,
			"use_unified_model": params.UseUnifiedModel,
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
