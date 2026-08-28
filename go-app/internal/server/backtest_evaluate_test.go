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
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
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
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, int64(111), payload.Match.MatchID)
	require.Equal(t, "T20", payload.Filters["format"])
	// Expected MAE: |25-30| + |15-10| + |1-0| = 5 + 5 + 1 = 11; /3 = 3.6666...
	mae, ok := payload.Metrics["player_runs_mae"]
	require.True(t, ok)
	require.InDelta(t, 3.6667, mae, 0.01)
	require.NotEmpty(t, payload.Players)
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
	mlBacktestPredictFunc = func(_ context.Context, cutoffArg time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
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
	require.True(t, receivedCutoff.Equal(cutoff), "ml cutoff = %v, want %v", receivedCutoff, cutoff)
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
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
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
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))

	// Check summary metrics
	wktsMAE, ok := payload.Metrics["player_wickets_mae"]
	require.True(t, ok)
	// Abs errors: |1-2|=1, |0-0|=0 -> (1+0)/2 = 0.5
	require.InDelta(t, 0.5, wktsMAE, 0.01)

	econMAE, ok := payload.Metrics["player_economy_mae"]
	require.True(t, ok)
	// Abs errors: |8.0-7.5|=0.5, |5.5-6.0|=0.5 -> (0.5+0.5)/2 = 0.5
	require.InDelta(t, 0.5, econMAE, 0.01)

	require.Len(t, payload.Players, 2)
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
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
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
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))

	// Summary metrics
	// catches abs errors: |1-2|=1, |0-0|=0 -> 0.5
	require.InDelta(t, 0.5, payload.Metrics["player_catches_mae"], 0.01)
	// run_outs abs errors: |2-1|=1, |1-0|=1 -> 1.0
	require.InDelta(t, 1.0, payload.Metrics["player_run_outs_mae"], 0.01)
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
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
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
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	// Check match_aggregates presence (predicted = sum of player preds, no baseline)
	require.NotNil(t, payload.MatchAggregates.Predicted)
	require.NotNil(t, payload.MatchAggregates.Actual)
	require.NotNil(t, payload.MatchAggregates.Errors)
	// Predicted runs = 18+35 = 53, wickets = 0, extras = 0. Actual: 150, 7, 10.
	require.InDelta(t, 97.0, payload.Metrics["match_runs_mae"], 0.1)
	require.InDelta(t, 7.0, payload.Metrics["match_wickets_mae"], 0.1)
	require.InDelta(t, 10.0, payload.Metrics["match_extras_mae"], 0.1)
	// winner_accuracy: pred winner from team run sums; without DB GetMatchPlayerTeams returns empty, so pred winner "" -> 0
	require.Equal(t, float64(0), payload.Metrics["winner_accuracy"])
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
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
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
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	// Predicted runs = 9 (single player), wickets = 0, extras = 0
	require.InDelta(t, 9.0, payload.MatchAggregates.Predicted["runs"].(float64), 0.1)
	require.Equal(t, float64(0), payload.MatchAggregates.Predicted["wickets"].(float64))
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
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
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
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	rmse, ok := payload.Metrics["player_runs_rmse"]
	require.True(t, ok)
	// Expected diffs: -5, +5, +1 ⇒ MSE = (25+25+1)/3 = 17 ⇒ RMSE ≈ 4.1231
	require.InDelta(t, 4.123, rmse, 0.01)
	r2, ok := payload.Metrics["player_runs_r2"]
	require.True(t, ok)
	// With actuals {30,10,0} and preds {25,15,1}, R² ≈ 0.8905
	require.InDelta(t, 0.89, r2, 0.02)
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
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
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
	require.True(t, gotCutoff.Equal(cutoff), "features cutoff mismatch")
	require.Len(t, gotIDs, len(squadIDs))
}
