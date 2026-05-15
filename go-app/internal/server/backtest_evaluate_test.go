package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test evaluate mode happy path with minimal targets (runs) and MAE computation
func TestBacktestMatchHandler_EvaluateMode_Success(t *testing.T) {
	// Backup seams and restore after
	origGetDate := getBacktestMatchDateFunc
	origGetSquads := getBacktestSquadPlayerIDsFunc
	origGetFeats := getBacktestFeaturesAtCutoffFunc
	origGetActuals := getBacktestPlayerActualsForMatchFunc
	origML := mlBacktestPredictFunc
	defer func() {
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquads
		getBacktestFeaturesAtCutoffFunc = origGetFeats
		getBacktestPlayerActualsForMatchFunc = origGetActuals
		mlBacktestPredictFunc = origML
	}()

	// Stub seams
	cutoff := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)
	getBacktestMatchDateFunc = func(_ context.Context, matchID int64) (time.Time, error) {
  require.Equal(t, int64(111), matchID)
		return cutoff, nil
	}
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) {
		return []int64{1, 2, 3}, nil
	}
	getBacktestFeaturesAtCutoffFunc = func(_ context.Context, _ time.Time, _ []int64, _ int64, _ string) (map[int64]map[string]float64, error) {
		return map[int64]map[string]float64{1: {}, 2: {}, 3: {}}, nil
	}
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return map[int64]playerActuals{
			1: {Runs: 30},
			2: {Runs: 10},
			3: {Runs: 0},
		}, nil
	}
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
		return map[int64]playerPredictions{
			1: {Runs: 25},
			2: {Runs: 15},
			3: {Runs: 1},
		}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&match_id=111", nil)

	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var payload backtestEvaluateResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
 require.Equal(t, int64(111), payload.Match.MatchID)
	if payload.Filters["format"] != "T20" {
		t.Fatalf("filters.format = %v, want T20", payload.Filters["format"])
	}
	// Expected MAE: |25-30| + |15-10| + |1-0| = 5 + 5 + 1 = 11; /3 = 3.6666...
	mae, ok := payload.Metrics["player_runs_mae"]
	require.True(t, ok)
	if mae < 3.66 || mae > 3.67 {
		t.Fatalf("player_runs_mae = %f, want ~3.6667", mae)
	}
	if len(payload.Players) == 0 {
		t.Fatalf("players empty, expected rows")
	}
}

func TestBacktestMatchHandler_EvaluateMode_MissingMatchID(t *testing.T) {
	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	// mode will be evaluate because we set it, but match_id missing
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&mode=evaluate", nil)
	app.backtestMatchHandler(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

// Ensure handler passes strict cutoff (match date) through to ML seam
func TestBacktestMatchHandler_EvaluateMode_PassesCutoffToML(t *testing.T) {
	// Backup seams and restore after
	origGetDate := getBacktestMatchDateFunc
	origGetSquads := getBacktestSquadPlayerIDsFunc
	origGetFeats := getBacktestFeaturesAtCutoffFunc
	origGetActuals := getBacktestPlayerActualsForMatchFunc
	origML := mlBacktestPredictFunc
	defer func() {
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquads
		getBacktestFeaturesAtCutoffFunc = origGetFeats
		getBacktestPlayerActualsForMatchFunc = origGetActuals
		mlBacktestPredictFunc = origML
	}()

	cutoff := time.Date(2023, 7, 15, 10, 30, 0, 0, time.UTC)
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
		return cutoff, nil
	}
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) {
		return []int64{10, 20}, nil
	}
	getBacktestFeaturesAtCutoffFunc = func(_ context.Context, _ time.Time, _ []int64, _ int64, _ string) (map[int64]map[string]float64, error) {
		return map[int64]map[string]float64{10: {}, 20: {}}, nil
	}
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return map[int64]playerActuals{
			10: {Runs: 40},
			20: {Runs: 20},
		}, nil
	}

	var receivedCutoff time.Time
	mlBacktestPredictFunc = func(_ context.Context, cutoffArg time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
		receivedCutoff = cutoffArg
		return map[int64]playerPredictions{
			10: {Runs: 35},
			20: {Runs: 25},
		}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&match_id=222", nil)
	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	if !receivedCutoff.Equal(cutoff) {
		t.Fatalf("ml cutoff = %v, want %v", receivedCutoff, cutoff)
	}
}

// Validate bowling metrics (wickets, economy) are included when available and MAE is computed.
func TestBacktestMatchHandler_EvaluateMode_BowlingMetrics(t *testing.T) {
	// Backup seams and restore after
	origGetDate := getBacktestMatchDateFunc
	origGetSquads := getBacktestSquadPlayerIDsFunc
	origGetFeats := getBacktestFeaturesAtCutoffFunc
	origGetActuals := getBacktestPlayerActualsForMatchFunc
	origML := mlBacktestPredictFunc
	defer func() {
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquads
		getBacktestFeaturesAtCutoffFunc = origGetFeats
		getBacktestPlayerActualsForMatchFunc = origGetActuals
		mlBacktestPredictFunc = origML
	}()

	cutoff := time.Date(2024, 11, 5, 9, 0, 0, 0, time.UTC)
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
		return cutoff, nil
	}
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) {
		return []int64{101, 102}, nil
	}
	getBacktestFeaturesAtCutoffFunc = func(_ context.Context, _ time.Time, _ []int64, _ int64, _ string) (map[int64]map[string]float64, error) {
		return map[int64]map[string]float64{101: {}, 102: {}}, nil
	}
	// Actuals: include runs, wickets, economy
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return map[int64]playerActuals{
			101: {Runs: 30, Wickets: 2, Economy: 7.5},
			102: {Runs: 5, Wickets: 0, Economy: 6.0},
		}, nil
	}
	// Predictions
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
		return map[int64]playerPredictions{
			101: {Runs: 28, Wickets: 1, Economy: 8.0},
			102: {Runs: 10, Wickets: 0, Economy: 5.5},
		}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&match_id=999", nil)

	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var payload backtestEvaluateResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Check summary metrics
	wktsMAE, ok := payload.Metrics["player_wickets_mae"]
	require.True(t, ok)
	// Abs errors: |1-2|=1, |0-0|=0 -> (1+0)/2 = 0.5
	if wktsMAE < 0.49 || wktsMAE > 0.51 {
		t.Fatalf("player_wickets_mae = %f, want ~0.5", wktsMAE)
	}

	econMAE, ok := payload.Metrics["player_economy_mae"]
	require.True(t, ok)
	// Abs errors: |8.0-7.5|=0.5, |5.5-6.0|=0.5 -> (0.5+0.5)/2 = 0.5
	if econMAE < 0.49 || econMAE > 0.51 {
		t.Fatalf("player_economy_mae = %f, want ~0.5", econMAE)
	}

	if len(payload.Players) != 2 {
		t.Fatalf("players len = %d, want 2", len(payload.Players))
	}
}

// Validate fielding metrics (catches, run_outs) are included when available and MAE is computed.
func TestBacktestMatchHandler_EvaluateMode_FieldingMetrics(t *testing.T) {
	// Backup seams and restore after
	origGetDate := getBacktestMatchDateFunc
	origGetSquads := getBacktestSquadPlayerIDsFunc
	origGetFeats := getBacktestFeaturesAtCutoffFunc
	origGetActuals := getBacktestPlayerActualsForMatchFunc
	origML := mlBacktestPredictFunc
	defer func() {
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquads
		getBacktestFeaturesAtCutoffFunc = origGetFeats
		getBacktestPlayerActualsForMatchFunc = origGetActuals
		mlBacktestPredictFunc = origML
	}()

	cutoff := time.Date(2024, 11, 6, 9, 0, 0, 0, time.UTC)
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) { return cutoff, nil }
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) {
		return []int64{201, 202}, nil
	}
	getBacktestFeaturesAtCutoffFunc = func(_ context.Context, _ time.Time, _ []int64, _ int64, _ string) (map[int64]map[string]float64, error) {
		return map[int64]map[string]float64{201: {}, 202: {}}, nil
	}
	// Actuals: include fielding catches and run_outs
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return map[int64]playerActuals{
			201: {Runs: 10, Catches: 2, RunOuts: 1},
			202: {Runs: 5, Catches: 0, RunOuts: 0},
		}, nil
	}
	// Predictions include fielding
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
		return map[int64]playerPredictions{
			201: {Runs: 12, Catches: 1, RunOuts: 2},
			202: {Runs: 4, Catches: 0, RunOuts: 1},
		}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&match_id=1001", nil)

	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var payload backtestEvaluateResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Summary metrics
	// catches abs errors: |1-2|=1, |0-0|=0 -> 0.5
	if got := payload.Metrics["player_catches_mae"]; got < 0.49 || got > 0.51 {
		t.Fatalf("player_catches_mae = %v, want ~0.5", got)
	}
	// run_outs abs errors: |2-1|=1, |1-0|=1 -> 1.0
	if got := payload.Metrics["player_run_outs_mae"]; got < 0.99 || got > 1.01 {
		t.Fatalf("player_run_outs_mae = %v, want ~1.0", got)
	}
}

// Match-level aggregates: verify response fields and summary metrics
func TestBacktestMatchHandler_EvaluateMode_MatchAggregatesMetrics(t *testing.T) {
	// Backup seams and restore after
	origGetDate := getBacktestMatchDateFunc
	origGetSquads := getBacktestSquadPlayerIDsFunc
	origGetFeats := getBacktestFeaturesAtCutoffFunc
	origGetActuals := getBacktestPlayerActualsForMatchFunc
	origMLPlayers := mlBacktestPredictFunc
	origAggActuals := getBacktestMatchAggregatesActualsFunc
	defer func() {
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquads
		getBacktestFeaturesAtCutoffFunc = origGetFeats
		getBacktestPlayerActualsForMatchFunc = origGetActuals
		mlBacktestPredictFunc = origMLPlayers
		getBacktestMatchAggregatesActualsFunc = origAggActuals
	}()

	cutoff := time.Date(2024, 12, 1, 12, 0, 0, 0, time.UTC)
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
		return cutoff, nil
	}
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) {
		return []int64{1, 2}, nil
	}
	getBacktestFeaturesAtCutoffFunc = func(_ context.Context, _ time.Time, _ []int64, _ int64, _ string) (map[int64]map[string]float64, error) {
		return map[int64]map[string]float64{1: {}, 2: {}}, nil
	}
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return map[int64]playerActuals{
			1: {Runs: 20},
			2: {Runs: 30},
		}, nil
	}
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
		return map[int64]playerPredictions{
			1: {Runs: 18},
			2: {Runs: 35},
		}, nil
	}
	// Match aggregates: predicted from player predictions (18+35=53 runs, 0 wickets, 0 extras); no ML match-aggregates call.
	getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
		return matchAggregates{Runs: 150, Wickets: 7, Extras: 10, WinnerTeamCode: "IND"}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&match_id=555", nil)

	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var payload backtestEvaluateResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Check match_aggregates presence (predicted = sum of player preds, no baseline)
	if payload.MatchAggregates.Predicted == nil || payload.MatchAggregates.Actual == nil ||
		payload.MatchAggregates.Errors == nil {
		t.Fatalf("match_aggregates missing sections")
	}
	// Predicted runs = 18+35 = 53, wickets = 0, extras = 0. Actual: 150, 7, 10.
	if got := payload.Metrics["match_runs_mae"]; got < 96.9 || got > 97.1 {
		t.Fatalf("match_runs_mae = %v, want ~97 (|53-150|)", got)
	}
	if got := payload.Metrics["match_wickets_mae"]; got < 6.9 || got > 7.1 {
		t.Fatalf("match_wickets_mae = %v, want ~7", got)
	}
	if got := payload.Metrics["match_extras_mae"]; got < 9.9 || got > 10.1 {
		t.Fatalf("match_extras_mae = %v, want ~10", got)
	}
	// winner_accuracy: pred winner from team run sums; without DB GetMatchPlayerTeams returns empty, so pred winner "" -> 0
	if got := payload.Metrics["winner_accuracy"]; got != 0 {
		t.Fatalf("winner_accuracy = %v, want 0 (no team assignment in test)", got)
	}
}

// Ensure match aggregates are derived from player predictions (no ML match-aggregates baseline).
func TestBacktestMatchHandler_EvaluateMode_MatchAggregates_FromPlayerPreds(t *testing.T) {
	origGetDate := getBacktestMatchDateFunc
	origGetSquads := getBacktestSquadPlayerIDsFunc
	origGetFeats := getBacktestFeaturesAtCutoffFunc
	origGetActuals := getBacktestPlayerActualsForMatchFunc
	origMLPlayers := mlBacktestPredictFunc
	origAggActuals := getBacktestMatchAggregatesActualsFunc
	defer func() {
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquads
		getBacktestFeaturesAtCutoffFunc = origGetFeats
		getBacktestPlayerActualsForMatchFunc = origGetActuals
		mlBacktestPredictFunc = origMLPlayers
		getBacktestMatchAggregatesActualsFunc = origAggActuals
	}()

	cutoff := time.Date(2025, 1, 2, 8, 0, 0, 0, time.UTC)
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) { return cutoff, nil }
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1}, nil }
	getBacktestFeaturesAtCutoffFunc = func(_ context.Context, _ time.Time, _ []int64, _ int64, _ string) (map[int64]map[string]float64, error) {
		return map[int64]map[string]float64{1: {}}, nil
	}
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return map[int64]playerActuals{1: {Runs: 10}}, nil
	}
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
		return map[int64]playerPredictions{1: {Runs: 9}}, nil
	}
	getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
		return matchAggregates{Runs: 95, Wickets: 6, Extras: 6, WinnerTeamCode: "AUS"}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&match_id=777", nil)
	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var payload backtestEvaluateResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Predicted runs = 9 (single player), wickets = 0, extras = 0
	if pred, ok := payload.MatchAggregates.Predicted["runs"].(float64); !ok || pred < 8.9 || pred > 9.1 {
		t.Fatalf("predicted runs = %v, want 9", payload.MatchAggregates.Predicted["runs"])
	}
	if pred, ok := payload.MatchAggregates.Predicted["wickets"].(float64); !ok || pred != 0 {
		t.Fatalf("predicted wickets = %v, want 0", payload.MatchAggregates.Predicted["wickets"])
	}
}

// Verify added metrics: RMSE and R² for player runs are computed and reasonable.
func TestBacktestMatchHandler_EvaluateMode_RMSE_R2(t *testing.T) {
	// Backup seams and restore after
	origGetDate := getBacktestMatchDateFunc
	origGetSquads := getBacktestSquadPlayerIDsFunc
	origGetFeats := getBacktestFeaturesAtCutoffFunc
	origGetActuals := getBacktestPlayerActualsForMatchFunc
	origML := mlBacktestPredictFunc
	defer func() {
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquads
		getBacktestFeaturesAtCutoffFunc = origGetFeats
		getBacktestPlayerActualsForMatchFunc = origGetActuals
		mlBacktestPredictFunc = origML
	}()

	cutoff := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) { return cutoff, nil }
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1, 2, 3}, nil }
	getBacktestFeaturesAtCutoffFunc = func(_ context.Context, _ time.Time, _ []int64, _ int64, _ string) (map[int64]map[string]float64, error) {
		return map[int64]map[string]float64{1: {}, 2: {}, 3: {}}, nil
	}
	// Actuals
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return map[int64]playerActuals{
			1: {Runs: 30},
			2: {Runs: 10},
			3: {Runs: 0},
		}, nil
	}
	// Predictions
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
		return map[int64]playerPredictions{
			1: {Runs: 25},
			2: {Runs: 15},
			3: {Runs: 1},
		}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&match_id=111", nil)

	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var payload backtestEvaluateResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rmse, ok := payload.Metrics["player_runs_rmse"]
	require.True(t, ok)
	// Expected diffs: -5, +5, +1 ⇒ MSE = (25+25+1)/3 = 17 ⇒ RMSE ≈ 4.1231
	if rmse < 4.12 || rmse > 4.13 {
		t.Fatalf("player_runs_rmse = %f, want ~4.123", rmse)
	}
	r2, ok := payload.Metrics["player_runs_r2"]
	require.True(t, ok)
	// With actuals {30,10,0} and preds {25,15,1}, R² ≈ 0.8905
	if r2 < 0.88 || r2 > 0.91 {
		t.Fatalf("player_runs_r2 = %f, want ~0.89", r2)
	}
}

// Ensure the cutoff-aware features seam is invoked with the correct cutoff and player IDs.
func TestBacktestMatchHandler_EvaluateMode_FeaturesSeamCalled(t *testing.T) {
	// Backup seams and restore after
	origGetDate := getBacktestMatchDateFunc
	origGetSquads := getBacktestSquadPlayerIDsFunc
	origGetActuals := getBacktestPlayerActualsForMatchFunc
	origML := mlBacktestPredictFunc
	origFeats := getBacktestFeaturesAtCutoffFunc
	defer func() {
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquads
		getBacktestPlayerActualsForMatchFunc = origGetActuals
		mlBacktestPredictFunc = origML
		getBacktestFeaturesAtCutoffFunc = origFeats
	}()

	cutoff := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) { return cutoff, nil }
	squadIDs := []int64{7, 8, 9}
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return squadIDs, nil }
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return map[int64]playerActuals{7: {Runs: 10}, 8: {Runs: 20}, 9: {Runs: 30}}, nil
	}
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
		return map[int64]playerPredictions{7: {Runs: 11}, 8: {Runs: 19}, 9: {Runs: 31}}, nil
	}

	var called bool
	var gotCutoff time.Time
	var gotIDs []int64
	getBacktestFeaturesAtCutoffFunc = func(_ context.Context, cutoffArg time.Time, playerIDs []int64, _ int64, _ string) (map[int64]map[string]float64, error) {
		called = true
		gotCutoff = cutoffArg
		gotIDs = append([]int64{}, playerIDs...)
		// Return empty features map (test does not need match context)
		return map[int64]map[string]float64{}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS&match_id=313", nil)

	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.True(t, called)
	if !gotCutoff.Equal(cutoff) {
		t.Fatalf("features cutoff = %v, want %v", gotCutoff, cutoff)
	}
	if len(gotIDs) != len(squadIDs) {
		t.Fatalf("features playerIDs len = %d, want %d", len(gotIDs), len(squadIDs))
	}
}
