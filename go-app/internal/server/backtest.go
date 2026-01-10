package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// testing seam for DB call
var listPlayedByFmtTeams = db.ListPlayedMatchesByFormatAndTeams

// Evaluate-mode seams (to be backed by DB repos; overridden in tests)
var (
	// Returns the match date (cutoff) for the given match id
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
		// Placeholder: to be implemented via db repo in a later step
		return time.Time{}, sql.ErrNoRows
	}
	// Returns the list of player IDs who actually played the match (XI + subs if available)
	// Requires cutoff and optional format to align with DB query semantics
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) {
		return nil, sql.ErrNoRows
	}
	// Returns actuals for players in the match, keyed by player id; minimal target: runs
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return nil, sql.ErrNoRows
	}
	// Optional: retrieve features at-or-before cutoff; may be unused by tests initially
	getBacktestFeaturesAtCutoffFunc = func(ctx context.Context, cutoff time.Time, playerIDs []int64) (map[int64]map[string]float64, error) {
		// Wire to default DB-based provider; tests may override this seam
		return db.DefaultFeatureProviderInst.GetPlayerFeaturesAtCutoff(ctx, cutoff, playerIDs)
	}
	// ML seam for backtest: given cutoff and player ids, return predicted targets per player
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
		return nil, sql.ErrNoRows
	}
	// Match-level aggregates: actuals from DB for given match
	getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
		return matchAggregates{}, sql.ErrNoRows
	}
    // Match-level aggregates: predictions from ML given cutoff and teams, with model version
    mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
        return matchAggregates{}, "", sql.ErrNoRows
    }
)

// dashboard accuracy-trend seams (overridable in tests)
var listPlayedMatchesByFilters = func(
	ctx context.Context,
	format string,
	team1 string,
	team2 string,
	start time.Time,
	end time.Time,
	order string,
	limit int,
) ([]backtestCandidate, error) {
	// Delegate to DB repository implementation; transform DB rows to server DTO.
	rows, err := db.ListPlayedMatchesByFilters(ctx, format, team1, team2, start, end, order, limit)
	if err != nil {
		return nil, err
	}
	out := make([]backtestCandidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, backtestCandidate{
			MatchID:        r.MatchID,
			StableID:       nullString(r.StableID),
			Date:           r.Date.Format(time.RFC3339),
			Venue:          nullString(r.Venue),
			Season:         nullString(r.Season),
			Format:         nullString(r.FormatCode),
			Team1:          r.Team1,
			Team2:          r.Team2,
			WinnerTeamCode: nullString(r.WinnerTeam),
		})
	}
	return out, nil
}

type backtestSelectResponse struct {
	Filters    map[string]any      `json:"filters"`
	Candidates []backtestCandidate `json:"candidates"`
}

type backtestCandidate struct {
	MatchID        int64  `json:"match_id"`
	StableID       string `json:"stable_id"`
	Date           string `json:"date"`
	Venue          string `json:"venue"`
	Season         string `json:"season"`
	Format         string `json:"format"`
	Team1          string `json:"team1"`
	Team2          string `json:"team2"`
	WinnerTeamCode string `json:"winner_team_code"`
}

// Evaluate-mode DTOs
type playerPredictions struct {
	Runs    float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

type playerActuals struct {
	Runs    float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

// Internal struct for match-level aggregates
type matchAggregates struct {
	Runs           float64
	Wickets        float64
	Extras         float64
	WinnerTeamCode string
}

type backtestEvaluateResponse struct {
	Filters map[string]any `json:"filters"`
	Match   struct {
		MatchID int64  `json:"match_id"`
		Date    string `json:"date"`
	} `json:"match"`
	MatchAggregates struct {
		Predicted map[string]any     `json:"predicted"`
		Actual    map[string]any     `json:"actual"`
		Errors    map[string]float64 `json:"errors"`
	} `json:"match_aggregates,omitempty"`
	Players []struct {
		PlayerID  int64              `json:"player_id"`
		Predicted map[string]float64 `json:"predicted"`
		Actual    map[string]float64 `json:"actual"`
		Errors    map[string]float64 `json:"errors"`
	} `json:"players"`
	Metrics map[string]float64 `json:"metrics"`
}

// Accuracy trend DTOs
type accuracyTrendItem struct {
	MatchID int64              `json:"match_id"`
	Date    string             `json:"date"`
	Format  string             `json:"format"`
	Team1   string             `json:"team1"`
	Team2   string             `json:"team2"`
	Metrics map[string]float64 `json:"metrics"`
}

type accuracyTrendResponse struct {
	Filters     map[string]any       `json:"filters"`
	Count       int                  `json:"count"`
	Results     []accuracyTrendItem  `json:"results"`
	Summary     map[string]float64   `json:"summary"`
	Progressive []map[string]float64 `json:"progressive"`
}

// --- Accuracy Trend: Helpers & Refactor Support ---

// Parameter parsing errors (used to keep handler responses identical)
var (
	errInvalidStartDate = errors.New("invalid start_date")
	errInvalidEndDate   = errors.New("invalid end_date")
	errEndBeforeStart   = errors.New("end_date before start_date")
)

type accuracyTrendParams struct {
	Format string
	Team1  string
	Team2  string
	Order  string
	Limit  int
	Cache  string
	// Metrics selection
	IncludePlayer bool
	IncludeTeam   bool
	Start         time.Time
	End           time.Time
	HasStart      bool
	HasEnd        bool
	RawStart      string
	RawEnd        string
}

func parseBacktestAccuracyTrendParams(r *http.Request) (accuracyTrendParams, error) {
	q := r.URL.Query()
	out := accuracyTrendParams{
		Format:   strings.TrimSpace(q.Get("format")),
		Team1:    strings.TrimSpace(q.Get("team1")),
		Team2:    strings.TrimSpace(q.Get("team2")),
		Order:    strings.TrimSpace(q.Get("order")),
		Cache:    strings.TrimSpace(q.Get("cache")),
		RawStart: strings.TrimSpace(q.Get("start_date")),
		RawEnd:   strings.TrimSpace(q.Get("end_date")),
	}
	if out.Order == "" {
		out.Order = "asc"
	}
	switch strings.ToLower(out.Cache) {
	case "", "readwrite":
		out.Cache = "readwrite"
	case "read":
		out.Cache = "read"
	case "off":
		out.Cache = "off"
	default:
		out.Cache = "readwrite"
	}
	if s := strings.TrimSpace(q.Get("limit")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			out.Limit = v
		}
	}
	// Enforce default limit and max cap
	if out.Limit <= 0 {
		out.Limit = 100
	}
	if out.Limit > 500 {
		out.Limit = 500
	}
	// Parse dates in YYYY-MM-DD
	if out.RawStart != "" {
		t, err := time.Parse("2006-01-02", out.RawStart)
		if err != nil {
			return accuracyTrendParams{}, errInvalidStartDate
		}
		out.Start, out.HasStart = t, true
	}
	if out.RawEnd != "" {
		t, err := time.Parse("2006-01-02", out.RawEnd)
		if err != nil {
			return accuracyTrendParams{}, errInvalidEndDate
		}
		out.End, out.HasEnd = t, true
	}
	if out.HasStart && out.HasEnd && out.End.Before(out.Start) {
		return accuracyTrendParams{}, errEndBeforeStart
	}

	// Parse metrics selection
	metrics := strings.TrimSpace(q.Get("metrics"))
	if metrics == "" {
		out.IncludePlayer = true
		out.IncludeTeam = true
	} else {
		parts := strings.Split(metrics, ",")
		for _, p := range parts {
			switch strings.ToLower(strings.TrimSpace(p)) {
			case "player":
				out.IncludePlayer = true
			case "team":
				out.IncludeTeam = true
			}
		}
		// If user passed unknown tokens, default to both to be safe
		if !out.IncludePlayer && !out.IncludeTeam {
			out.IncludePlayer = true
			out.IncludeTeam = true
		}
	}
	return out, nil
}

// Cache seams for match-level aggregates (overridable in tests)
var (
	getMatchPredictionAggregatesFunc    = db.GetMatchPredictionAggregates
	upsertMatchPredictionAggregatesFunc = db.UpsertMatchPredictionAggregates
)

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
	cutoff, cerr := getBacktestMatchDateFunc(ctx, m.MatchID)
	if cerr != nil || cutoff.IsZero() {
		if t, err := time.Parse(time.RFC3339, m.Date); err == nil {
			cutoff = t
		}
	}
	if cutoff.IsZero() {
		return metrics
	}

	// Player-level MAE on runs (optional)
	if includePlayer {
		if squad, err := getBacktestSquadPlayerIDsFunc(ctx, m.MatchID, cutoff, m.Format); err == nil && len(squad) > 0 {
			preds, err1 := mlBacktestPredictFunc(ctx, cutoff, squad)
			acts, err2 := getBacktestPlayerActualsForMatchFunc(ctx, m.MatchID)
			if err1 == nil && err2 == nil {
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
					metrics["player_runs_mae"] = totalAbs / cnt
				}
			}
		}
	}

	// Match/team aggregates with optional cache (optional)
	if includeTeam && m.Team1 != "" && m.Team2 != "" {
		var predAgg matchAggregates
		var havePred bool

		if cacheMode == "read" || cacheMode == "readwrite" {
			if rec, err := getMatchPredictionAggregatesFunc(ctx, m.MatchID); err == nil {
				predAgg = matchAggregates{
					Runs:           rec.PredictedTotalRuns.Float64,
					WinnerTeamCode: rec.PredictedWinnerCode.String,
				}
				havePred = true
			}
		}

  if !havePred {
            if p, modelVersion, err := mlBacktestPredictMatchAggregatesFunc(ctx, cutoff, [2]string{m.Team1, m.Team2}); err == nil {
                predAgg = p
                havePred = true
                if cacheMode == "readwrite" {
                    if err := upsertMatchPredictionAggregatesFunc(ctx, db.MatchPredictionAggregates{
                        MatchID:             m.MatchID,
                        Format:              m.Format,
                        Team1Code:           m.Team1,
                        Team2Code:           m.Team2,
                        PredictedWinnerCode: sqlNullString(predAgg.WinnerTeamCode),
                        PredictedTotalRuns:  sqlNullFloat64(predAgg.Runs),
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
			}
		}

		if havePred {
			if actAgg, errA := getBacktestMatchAggregatesActualsFunc(ctx, m.MatchID); errA == nil {
				if actAgg.Runs != 0 || predAgg.Runs != 0 {
					metrics["team_runs_mae"] = math.Abs(predAgg.Runs - actAgg.Runs)
				}
				if actAgg.WinnerTeamCode != "" && predAgg.WinnerTeamCode != "" {
					if strings.EqualFold(actAgg.WinnerTeamCode, predAgg.WinnerTeamCode) {
						metrics["team_winner_accuracy"] = 1.0
					} else {
						metrics["team_winner_accuracy"] = 0.0
					}
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

// helpers to construct sql nullable types
func sqlNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func sqlNullFloat64(v float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: v, Valid: true}
}

// backtestMatchHandler handles GET /api/backtest/match
// Modes:
// - select (default when match_id is absent): returns candidate played matches for given filters
// - evaluate (when match_id present): to be implemented in a later step
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
		Metrics: map[string]float64{},
	}
	resp.Match.MatchID = mid
	resp.Match.Date = cutoff.Format(time.RFC3339)

	// Player rows and MAE for runs
	var (
		totalAbsErrRuns    float64
		countRuns          float64
		totalSqErrRuns     float64
		totalAbsErrWickets float64
		countWickets       float64
		totalAbsErrEcon    float64
		countEcon          float64
		totalAbsErrCatches float64
		countCatches       float64
		totalAbsErrRunOuts float64
		countRunOuts       float64
	)
	// Collect actuals for runs to compute RMSE and R² after loop
	runsActuals := make([]float64, 0, len(squad))
	for _, pid := range squad {
		pPred, okPred := preds[pid]
		pAct, okAct := actuals[pid]
		if !okPred || !okAct {
			// Skip players without both pred and actual
			continue
		}
		// Runs
		diffRuns := pPred.Runs - pAct.Runs
		absErrRuns := math.Abs(diffRuns)
		totalAbsErrRuns += absErrRuns
		countRuns++
		totalSqErrRuns += diffRuns * diffRuns
		runsActuals = append(runsActuals, pAct.Runs)

		// Wickets: always include when both pred and actual exist
		absErrWkts := math.Abs(pPred.Wickets - pAct.Wickets)
		totalAbsErrWickets += absErrWkts
		countWickets++

		// Economy: always include
		absErrEcon := math.Abs(pPred.Economy - pAct.Economy)
		totalAbsErrEcon += absErrEcon
		countEcon++

		// Fielding: catches: always include
		absErrCatches := math.Abs(pPred.Catches - pAct.Catches)
		totalAbsErrCatches += absErrCatches
		countCatches++

		// Fielding: run_outs: always include
		absErrRunOuts := math.Abs(pPred.RunOuts - pAct.RunOuts)
		totalAbsErrRunOuts += absErrRunOuts
		countRunOuts++

		row := struct {
			PlayerID  int64              `json:"player_id"`
			Predicted map[string]float64 `json:"predicted"`
			Actual    map[string]float64 `json:"actual"`
			Errors    map[string]float64 `json:"errors"`
		}{
			PlayerID:  pid,
			Predicted: map[string]float64{"runs": pPred.Runs},
			Actual:    map[string]float64{"runs": pAct.Runs},
			Errors:    map[string]float64{"runs_mae": absErrRuns},
		}
		// Enrich with bowling metrics if present (non-zero counts handled above)
		if pPred.Wickets != 0 || pAct.Wickets != 0 {
			row.Predicted["wickets"] = pPred.Wickets
			row.Actual["wickets"] = pAct.Wickets
			row.Errors["wickets_mae"] = absErrWkts
		}
		if pPred.Economy != 0 || pAct.Economy != 0 {
			row.Predicted["economy"] = pPred.Economy
			row.Actual["economy"] = pAct.Economy
			row.Errors["economy_mae"] = absErrEcon
		}
		if pPred.Catches != 0 || pAct.Catches != 0 {
			row.Predicted["catches"] = pPred.Catches
			row.Actual["catches"] = pAct.Catches
			row.Errors["catches_mae"] = absErrCatches
		}
		if pPred.RunOuts != 0 || pAct.RunOuts != 0 {
			row.Predicted["run_outs"] = pPred.RunOuts
			row.Actual["run_outs"] = pAct.RunOuts
			row.Errors["run_outs_mae"] = absErrRunOuts
		}
		resp.Players = append(resp.Players, row)
	}
	if countRuns > 0 {
		resp.Metrics["player_runs_mae"] = totalAbsErrRuns / countRuns
		// RMSE for runs
		resp.Metrics["player_runs_rmse"] = math.Sqrt(totalSqErrRuns / countRuns)
		// R² for runs: 1 - SS_res / SS_tot
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
			// Degenerate case: all actuals equal → define R² as 0 to avoid NaN
			resp.Metrics["player_runs_r2"] = 0.0
		}
	} else {
		resp.Metrics["player_runs_mae"] = 0
		resp.Metrics["player_runs_rmse"] = 0
		resp.Metrics["player_runs_r2"] = 0
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
 if predAgg, modelVersion, err1 := mlBacktestPredictMatchAggregatesFunc(r.Context(), cutoff, [2]string{team1, team2}); err1 == nil {
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

func nullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// Wire default implementations for evaluate-mode seams to DB repos where available.
func init() {
	getBacktestMatchDateFunc = func(ctx context.Context, matchID int64) (time.Time, error) {
		d, err := db.GetMatchDate(ctx, matchID)
		if err != nil {
			return time.Time{}, err
		}
		if d == nil {
			return time.Time{}, sql.ErrNoRows
		}
		return *d, nil
	}
	// Minimal default for squad player IDs: read from player_match for the given match_id.
	getBacktestSquadPlayerIDsFunc = func(ctx context.Context, matchID int64, _ time.Time, _ string) ([]int64, error) {
		if db.Pool == nil {
			return nil, errors.New("db pool not initialized")
		}
		rows, err := db.Pool.Query(
			ctx,
			`SELECT DISTINCT player_id FROM player_match WHERE match_id = $1 ORDER BY player_id ASC`,
			matchID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		ids := make([]int64, 0, 22)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return ids, nil
	}
	// Actuals for match: optimized single-query LEFT JOIN across batting, bowling, fielding
	getBacktestPlayerActualsForMatchFunc = func(ctx context.Context, matchID int64) (map[int64]playerActuals, error) {
		if db.Pool == nil {
			return nil, errors.New("db pool not initialized")
		}
		// Unified query to reduce DB round-trips: get all player actuals with LEFT JOINs
		rows, err := db.Pool.Query(
			ctx,
			`
         SELECT
             pm.player_id,
             COALESCE(bd.runs, 0) AS runs,
             COALESCE(bw.wickets, 0) AS wickets,
             COALESCE(bw.econ, 0) AS econ,
             COALESCE(fd.catches, 0) AS catches,
             COALESCE(fd.run_outs, 0) AS run_outs
         FROM player_match pm
         LEFT JOIN batting_data bd ON bd.match_id = pm.match_id AND bd.player_id = pm.player_id
         LEFT JOIN bowling_data bw ON bw.match_id = pm.match_id AND bw.player_id = pm.player_id
         LEFT JOIN fielding_data fd ON fd.match_id = pm.match_id AND fd.player_id = pm.player_id
         WHERE pm.match_id = $1
         ORDER BY pm.player_id ASC
         `,
			matchID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make(map[int64]playerActuals, 22)
		for rows.Next() {
			var (
				pid                                   int64
				runs, wickets, econ, catches, runOuts float64
			)
			if err := rows.Scan(&pid, &runs, &wickets, &econ, &catches, &runOuts); err != nil {
				return nil, err
			}
			out[pid] = playerActuals{
				Runs:    runs,
				Wickets: wickets,
				Economy: econ,
				Catches: catches,
				RunOuts: runOuts,
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}
	// Leave features/ML seams as placeholders; tests override them.
	// Wire default ML and aggregates seams to concrete clients/repos where available.
	// These can be overridden in tests.
	mlClient := NewBacktestMLClient()
	mlBacktestPredictFunc = func(ctx context.Context, cutoff time.Time, playerIDs []int64) (map[int64]playerPredictions, error) {
		// If mlClient is nil (should not happen), return placeholder error
		if mlClient == nil {
			return nil, errors.New("ml client not initialized")
		}
		return mlClient.predictPlayers(ctx, cutoff, playerIDs)
	}
 mlBacktestPredictMatchAggregatesFunc = func(ctx context.Context, cutoff time.Time, teams [2]string) (matchAggregates, string, error) {
        if mlClient == nil {
            return matchAggregates{}, "", errors.New("ml client not initialized")
        }
        return mlClient.predictMatchAggregates(ctx, cutoff, teams)
    }
	getBacktestMatchAggregatesActualsFunc = func(ctx context.Context, matchID int64) (matchAggregates, error) {
		ma, err := db.GetMatchAggregates(ctx, matchID)
		if err != nil {
			return matchAggregates{}, err
		}
		return matchAggregates{
			Runs:           ma.Runs,
			Wickets:        ma.Wickets,
			Extras:         ma.Extras,
			WinnerTeamCode: ma.WinnerTeamCode,
		}, nil
	}
}
