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
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	exq "github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// --- Helpers: thin delegations to services/backtest ---

func chooseBacktestMode(modeInput, matchID string) string {
	return backtest.ChooseBacktestMode(modeInput, matchID)
}

func computeR2(totalSquaredError float64, actuals []float64) float64 {
	return backtest.ComputeR2(totalSquaredError, actuals)
}

func winnerAccuracy(predWinner, actualWinner string) float64 {
	return backtest.WinnerAccuracy(predWinner, actualWinner)
}


func buildPredictedScorecard(
	actual *db.MatchScorecard,
	preds map[int64]playerPredictions,
	playerTeams map[int64]string,
) *db.MatchScorecard {
	return backtest.BuildPredictedScorecard(actual, preds, playerTeams)
}

func intPtr(n int) *int { return &n }

func float32Ptr(f float32) *float32 { return &f }

func computePlayerResultsAndMetrics(
	squad []int64,
	preds map[int64]playerPredictions,
	actuals map[int64]playerActuals,
) ([]BacktestPlayerResult, map[string]float64) {
	return backtest.ComputePlayerResultsAndMetrics(squad, preds, actuals)
}

func formatFromFilters(filters map[string]any) string {
	return backtest.FormatFromFilters(filters)
}

func buildBacktestWinFeatures(
	winCtx *db.MatchWinContext,
	format string,
	playerTeams map[int64]string,
	team1, team2 string,
	features map[int64]map[string]float64,
) predictteam.WinFeatures {
	return backtest.BuildBacktestWinFeatures(winCtx, format, playerTeams, team1, team2, features)
}

func rescaleBacktestPredictionsToWinProbability(
	resp *backtestEvaluateResponse,
	playerTeams map[int64]string,
	team1, team2 string,
	p float64,
) {
	backtest.RescalePredictionsToWinProbability(resp, playerTeams, team1, team2, p)
}

// populateMatchAggregatesAndMetrics fills the response with match-level predicted/actual aggregates
// and associated metrics. When features is non-nil, uses the win model for winner and rescales
// individual predicted runs (and wickets) so team totals match win probability.
func populateMatchAggregatesAndMetrics(
	ctx context.Context,
	resp *backtestEvaluateResponse,
	_ time.Time,
	team1 string,
	team2 string,
	matchID int64,
	features map[int64]map[string]float64,
) {
	playerTeams, err := db.GetMatchPlayerTeams(ctx, matchID)
	if err != nil {
		slog.WarnContext(ctx, "failed to get match player teams", slog.Int64("match_id", matchID), slog.Any("err", err))
		playerTeams = nil
	}
	if playerTeams == nil {
		playerTeams = make(map[int64]string)
	}

	// When win model is available and we have features, get win probability and rescale predictions.
	if len(features) > 0 {
		winCtx, err := db.GetMatchWinContext(ctx, matchID)
		if err != nil {
			slog.WarnContext(
				ctx,
				"failed to get match win context for rescaling",
				slog.Int64("match_id", matchID),
				slog.Any("err", err),
			)
		} else {
			w := buildBacktestWinFeatures(winCtx, formatFromFilters(resp.Filters), playerTeams, team1, team2, features)
			p, err := mlPredictMatchWinFunc(ctx, w)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					slog.WarnContext(ctx, "win model prediction failed during rescaling", slog.Int64("match_id", matchID), slog.Any("err", err))
				}
			} else if p >= 0 && p <= 1 {
				rescaleBacktestPredictionsToWinProbability(resp, playerTeams, team1, team2, p)
			}
		}
	}

	// Predicted: sum from (possibly rescaled) player predictions; winner from win model or team run totals.
	var predRuns, predWickets float64
	teamRuns := make(map[string]float64)
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
		slog.WarnContext(
			ctx,
			"failed to get match feature context for extras prediction",
			slog.Int64("match_id", matchID),
			slog.Any("err", err),
		)
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
	results, summary, progressive := computeAccuracyTrendForCandidates(
		r.Context(), candidates, params.IncludePlayer, params.IncludeTeam, params.Cache, params.UseUnifiedModel,
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

// backtestEvaluateStartHandler handles POST /api/backtest/evaluate-start (body or query: format, team1, team2, match_id).
// Starts evaluation in the background and returns { "job_id": "..." } so the client can poll evaluate-status.
func (a *App) backtestEvaluateStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST or GET required"})
		return
	}
	var format, team1, team2, matchID string
	var useUnifiedModel, useLatestModel bool
	useUnifiedFromBody, useLatestFromBody := false, false
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
		var body struct {
			Format          string `json:"format"`
			Team1           string `json:"team1"`
			Team2           string `json:"team2"`
			MatchID         int64  `json:"match_id"`
			UseUnifiedModel *bool  `json:"use_unified_model,omitempty"`
			UseLatestModel  *bool  `json:"use_latest_model,omitempty"`
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
		if body.UseUnifiedModel != nil {
			useUnifiedModel = *body.UseUnifiedModel
			useUnifiedFromBody = true
		}
		if body.UseLatestModel != nil {
			useLatestModel = *body.UseLatestModel
			useLatestFromBody = true
		}
	}
	if !useUnifiedFromBody {
		useUnifiedModel = parseUseUnifiedModel(r, false)
	}
	if !useLatestFromBody {
		useLatestModel = parseUseLatestModel(r, false)
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

	jobID, err := startEvaluateJob(r.Context(), format, team1, team2, matchID, useUnifiedModel, useLatestModel)
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

	useUnified := parseUseUnifiedModel(r, false)
	useLatest := parseUseLatestModel(r, false)
	resp, err := doEvaluateWork(r.Context(), format, team1, team2, matchID, useUnified, useLatest, progress)
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
	Innings  trainingDataPart `json:"innings"`
}

type trainingDataPart struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// allowedTrainingDataFormats is the fixed set of format codes safe to include in API error hints (avoids reflected input).
var allowedTrainingDataFormats = map[string]bool{
	"TEST": true, "ODI": true, "T20": true, "T20I": true, "all": true,
}

// respondTrainingDataErr maps known training-data errors to appropriate HTTP status and message.
// Format-not-found (e.g. migrations not run or match_format empty) -> 400; DB not ready -> 503; else 500.
func respondTrainingDataErr(w http.ResponseWriter, err error, format string) {
	if err == nil {
		return
	}
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
		hint := "Ensure migrations are applied and match_format is populated (TEST, ODI, T20, T20I)."
		if allowedTrainingDataFormats[format] {
			hint += " Format requested: " + format
		}
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "FORMAT_NOT_FOUND",
			Message: "format not found or database not ready for training-data",
			Hint:    hint,
		})
		slog.Info("training-data: format not found or no rows", slog.String("format", format), slog.Any("err", err))
		return
	}
	if strings.Contains(err.Error(), "db pool not initialized") {
		writeJSON(w, http.StatusServiceUnavailable, apiError{
			Code:    "SERVICE_UNAVAILABLE",
			Message: "database not connected",
			Hint:    "Go-app may still be starting; retry shortly.",
		})
		slog.Warn("training-data: db pool not initialized", slog.Any("err", err))
		return
	}
	respondErr(w, err)
}

// allowedTrainingDataSections is the set of valid section names for training-data ?sections= (reduces go-app/DB load when only one model is needed).
var allowedTrainingDataSections = map[string]bool{
	"batting": true, "bowling": true, "fielding": true, "extras": true, "win": true, "innings": true,
}

// backtestTrainingDataHandler handles GET /api/backtest/training-data?cutoff=...&format=...&sections=...
// cutoff (RFC3339) is required. format: use "all" (or omit) for all matches before cutoff; use a specific code (T20, ODI, etc.) to filter by that format.
// sections: optional comma-separated list (batting,bowling,fielding,extras,win). If omitted, all sections are returned (legacy). If set, only those sections are queried to reduce go-app and DB CPU during auto-tune.
func (a *App) backtestTrainingDataHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("training-data: panic recovered",
				slog.Any("panic", rec),
				slog.String("stack", string(debug.Stack())),
			)
			// Connection may already be broken; try to write 500
			w.WriteHeader(http.StatusInternalServerError)
		}
	}()

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
	formatForErr := format
	if formatForErr == "" {
		formatForErr = "all"
	}
	// Parse optional sections=batting,bowling,... so we only run the requested queries (reduces CPU when ml-service only needs one section).
	sectionsParam := strings.TrimSpace(r.URL.Query().Get("sections"))
	wantSection := map[string]bool{}
	if sectionsParam != "" {
		for _, s := range strings.Split(sectionsParam, ",") {
			s = strings.TrimSpace(strings.ToLower(s))
			if allowedTrainingDataSections[s] {
				wantSection[s] = true
			}
		}
	}
	runAllSections := len(wantSection) == 0

	slog.Info("training-data: request start",
		slog.String("cutoff", cutoffStr),
		slog.String("format", format),
		slog.String("sections", sectionsParam),
		slog.Bool("use_all", useAll),
		slog.Bool("run_all_sections", runAllSections),
	)

	var batRows, bowlRows, fieldRows, extrasRows, winRows, inningsRows [][]string
	type sectionLoader struct {
		name       string
		rows       *[][]string
		loadAll    func(context.Context, time.Time) ([][]string, error)
		loadFormat func(context.Context, string, time.Time) ([][]string, error)
	}
	loaders := []sectionLoader{
		{"batting", &batRows, exq.BattingTrainingRows, exq.BattingTrainingRowsWithFormat},
		{"bowling", &bowlRows, exq.BowlingTrainingRows, exq.BowlingTrainingRowsWithFormat},
		{"fielding", &fieldRows, exq.FieldingTrainingRows, exq.FieldingTrainingRowsWithFormat},
		{"extras", &extrasRows, exq.ExtrasTrainingRows, exq.ExtrasTrainingRowsWithFormat},
		{"win", &winRows, exq.WinTrainingRows, exq.WinTrainingRowsWithFormat},
		{"innings", &inningsRows, exq.InningsTrainingRows, exq.InningsTrainingRowsWithFormat},
	}
	for _, loader := range loaders {
		if runAllSections || wantSection[loader.name] {
			t0 := time.Now()
			var loadErr error
			if useAll {
				*loader.rows, loadErr = loader.loadAll(r.Context(), cutoff)
			} else {
				*loader.rows, loadErr = loader.loadFormat(r.Context(), format, cutoff)
			}
			elapsed := time.Since(t0)
			rowCount := len(*loader.rows)
			if loadErr != nil {
				slog.Error("training-data: section load failed",
					slog.String("section", loader.name),
					slog.Duration("elapsed", elapsed),
					slog.Any("err", loadErr),
				)
				respondTrainingDataErr(w, loadErr, formatForErr)
				return
			}
			slog.Info("training-data: section loaded",
				slog.String("section", loader.name),
				slog.Int("rows", rowCount),
				slog.Duration("elapsed_ms", elapsed),
			)
		}
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
	inningsH, inningsD := part(inningsRows)

	slog.Info("training-data: all sections ready, writing response",
		slog.Int("batting_rows", len(batD)),
		slog.Int("bowling_rows", len(bowlD)),
		slog.Int("fielding_rows", len(fieldD)),
		slog.Int("extras_rows", len(extrasD)),
		slog.Int("win_rows", len(winD)),
		slog.Int("innings_rows", len(inningsD)),
	)
	writeJSON(w, http.StatusOK, trainingDataResponse{
		Batting:  trainingDataPart{Headers: batH, Rows: batD},
		Bowling:  trainingDataPart{Headers: bowlH, Rows: bowlD},
		Fielding: trainingDataPart{Headers: fieldH, Rows: fieldD},
		Extras:   trainingDataPart{Headers: extrasH, Rows: extrasD},
		Win:      trainingDataPart{Headers: winH, Rows: winD},
		Innings:  trainingDataPart{Headers: inningsH, Rows: inningsD},
	})
	slog.Info("training-data: response written successfully")
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
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format is required (e.g. T20, ODI)"},
		)
		return
	}
	cfg := config.Load()
	limit := config.BacktestListDefaultLimit(cfg)
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
			maxLimit := config.BacktestListMaxLimit(cfg)
			if limit > maxLimit {
				limit = maxLimit
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
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format is required (e.g. T20, ODI)"},
		)
		return
	}
	cfg := config.Load()
	limit := config.BacktestListDefaultLimit(cfg)
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
			maxLimit := config.BacktestListMaxLimit(cfg)
			if limit > maxLimit {
				limit = maxLimit
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
