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
	// Match-level aggregates: predictions from ML given cutoff and teams
	mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, error) {
		return matchAggregates{}, sql.ErrNoRows
	}
)

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

  // Wickets (optional)
  var absErrWkts float64
  if pPred.Wickets != 0 || pAct.Wickets != 0 {
      absErrWkts = math.Abs(pPred.Wickets - pAct.Wickets)
      totalAbsErrWickets += absErrWkts
      countWickets++
  }

  // Economy (optional)
  var absErrEcon float64
  if pPred.Economy != 0 || pAct.Economy != 0 {
      absErrEcon = math.Abs(pPred.Economy - pAct.Economy)
      totalAbsErrEcon += absErrEcon
      countEcon++
  }

  // Fielding: catches (optional)
  var absErrCatches float64
  if pPred.Catches != 0 || pAct.Catches != 0 {
      absErrCatches = math.Abs(pPred.Catches - pAct.Catches)
      totalAbsErrCatches += absErrCatches
      countCatches++
  }

  // Fielding: run_outs (optional)
  var absErrRunOuts float64
  if pPred.RunOuts != 0 || pAct.RunOuts != 0 {
      absErrRunOuts = math.Abs(pPred.RunOuts - pAct.RunOuts)
      totalAbsErrRunOuts += absErrRunOuts
      countRunOuts++
  }

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
	if predAgg, err1 := mlBacktestPredictMatchAggregatesFunc(r.Context(), cutoff, [2]string{team1, team2}); err1 == nil {
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
	// Actuals for match: minimal implementation from batting_data (runs scored)
	getBacktestPlayerActualsForMatchFunc = func(ctx context.Context, matchID int64) (map[int64]playerActuals, error) {
		if db.Pool == nil {
			return nil, errors.New("db pool not initialized")
		}
		// Start with batting actuals (runs)
		rows, err := db.Pool.Query(ctx, `
            SELECT bd.player_id, COALESCE(bd.runs_scored,0)
            FROM batting_data bd
            WHERE bd.match_id = $1
        `, matchID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make(map[int64]playerActuals)
		for rows.Next() {
			var pid int64
			var runs float64
			if err := rows.Scan(&pid, &runs); err != nil {
				return nil, err
			}
			pa := out[pid]
			pa.Runs = runs
			out[pid] = pa
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		// Merge bowling actuals if available (wickets, economy)
		rows2, err := db.Pool.Query(ctx, `
            SELECT bw.player_id,
                   COALESCE(bw.wickets_taken,0),
                   COALESCE(bw.econ,0)
            FROM bowling_data bw
            WHERE bw.match_id = $1
        `, matchID)
		if err != nil {
			return nil, err
		}
		defer rows2.Close()
		for rows2.Next() {
			var pid int64
			var wickets, econ float64
			if err := rows2.Scan(&pid, &wickets, &econ); err != nil {
				return nil, err
			}
			pa := out[pid]
			pa.Wickets = wickets
			pa.Economy = econ
			out[pid] = pa
		}
		if err := rows2.Err(); err != nil {
			return nil, err
		}
		// Merge fielding actuals if available (catches, run_outs)
		rows3, err := db.Pool.Query(ctx, `
            SELECT fd.player_id,
                   COALESCE(fd.catches,0),
                   COALESCE(fd.run_outs,0)
            FROM fielding_data fd
            WHERE fd.match_id = $1
        `, matchID)
		if err != nil {
			return nil, err
		}
		defer rows3.Close()
		for rows3.Next() {
			var pid int64
			var catches, runOuts float64
			if err := rows3.Scan(&pid, &catches, &runOuts); err != nil {
				return nil, err
			}
			pa := out[pid]
			pa.Catches = catches
			pa.RunOuts = runOuts
			out[pid] = pa
		}
		if err := rows3.Err(); err != nil {
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
	mlBacktestPredictMatchAggregatesFunc = func(ctx context.Context, cutoff time.Time, teams [2]string) (matchAggregates, error) {
		if mlClient == nil {
			return matchAggregates{}, errors.New("ml client not initialized")
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
