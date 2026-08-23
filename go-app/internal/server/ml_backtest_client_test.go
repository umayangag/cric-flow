package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test PredictPlayers sends cutoff and player_ids and maps response correctly.
func TestBacktestMLClient_PredictPlayers(t *testing.T) {
	// Fake ML server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ml/backtest/predict", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		var payload struct {
			Cutoff    string  `json:"cutoff_date"`
			PlayerIDs []int64 `json:"player_ids"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		// Validate cutoff and ids propagated
		require.NotEmpty(t, payload.Cutoff)
		require.Equal(t, []int64{1, 2, 3}, payload.PlayerIDs)
		// Respond with deterministic predictions
		_ = json.NewEncoder(w).Encode(map[string]any{
			"players": []map[string]any{
				{"player_id": 1, "runs": 10.0, "wickets": 1.0, "economy": 6.5, "catches": 2.0, "run_outs": 1.0},
				{"player_id": 2, "runs": 20.0, "catches": 0.0},
				{"player_id": 3, "runs": 0.0},
			},
		})
	}))
	defer srv.Close()

	// Point client at fake server
	t.Setenv("ML_SERVICE_URL", srv.URL)
	c := NewBacktestMLClient()
	cutoff := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)
	res, err := c.predictPlayers(t.Context(), cutoff, "", []int64{1, 2, 3}, nil, false, nil)
	require.NoError(t, err)
	require.Len(t, res, 3)
	got := res[1]
	require.Equal(t, float64(10), got.Runs)
	require.Equal(t, float64(1), got.Wickets)
	require.Equal(t, 6.5, got.Economy)
	require.Equal(t, float64(2), got.Catches)
	require.Equal(t, float64(1), got.RunOuts)
}

// Test PredictMatchAggregates sends cutoff and teams and maps response correctly.
func TestBacktestMLClient_PredictMatchAggregates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ml/backtest/predict", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		var payload struct {
			Cutoff string    `json:"cutoff_date"`
			Teams  [2]string `json:"teams"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.NotEmpty(t, payload.Cutoff)
		require.Equal(t, [2]string{"IND", "AUS"}, payload.Teams)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"match": map[string]any{
				"runs":             160.0,
				"wickets":          6.0,
				"extras":           12.0,
				"winner_team_code": "IND",
			},
		})
	}))
	defer srv.Close()

	os.Setenv("ML_SERVICE_URL", srv.URL)
	c := NewBacktestMLClient()
	cutoff := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)
	agg, _, err := c.predictMatchAggregates(t.Context(), cutoff, [2]string{"IND", "AUS"})
	require.NoError(t, err)
	require.Equal(t, float64(160), agg.Runs)
	require.Equal(t, float64(6), agg.Wickets)
	require.Equal(t, float64(12), agg.Extras)
	require.Equal(t, "IND", agg.WinnerTeamCode)
}

// Test HistoricalMatchBacktest with match_id selector and with filters selector, plus validation error.
func TestBacktestMLClient_HistoricalMatchBacktest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ml/backtest/match", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		// Echo a fixed response body matching the ml-service schema
		_ = json.NewEncoder(w).Encode(map[string]any{
			"players": []map[string]any{
				{
					"player_id":         101,
					"predicted":         map[string]any{"runs": 20.0, "wickets": 1.0, "economy": 6.2},
					"actual":            map[string]any{"runs": 18.0, "wickets": 0.0, "economy": 7.0},
					"abs_error_runs":    2.0,
					"abs_error_wickets": 1.0,
				},
			},
			"match": map[string]any{
				"predicted": map[string]any{
					"runs": 160.0, "wickets": 6.0, "extras": 10.0, "winner_team_code": "IND",
				},
				"actual": map[string]any{
					"runs": 155.0, "wickets": 7.0, "extras": 12.0, "winner_team_code": "IND",
				},
			},
			"metrics": map[string]any{
				"mae_runs":       2.0,
				"rmse_runs":      2.0,
				"winner_correct": true,
			},
			"model_version": "v-test",
		})
	}))
	defer srv.Close()

	t.Setenv("ML_SERVICE_URL", srv.URL)
	c := NewBacktestMLClient()
	cutoff := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)

	// Case 1: by match_id
	mid := int64(789)
	res, err := c.historicalMatchBacktest(t.Context(), cutoff, &mid, nil)
	require.NoError(t, err)
	require.Equal(t, "v-test", res.ModelVersion)
	require.Len(t, res.Players, 1)
	require.Equal(t, "IND", res.Match.Predicted.WinnerTeamCode)
	require.Equal(t, float64(2), res.Metrics.MAERuns)

	// Case 2: by filters
	filters := &HistoricalMatchFilters{Format: "T20", Team1: "IND", Team2: "AUS", MatchDate: cutoff}
	res2, err := c.historicalMatchBacktest(t.Context(), cutoff, nil, filters)
	require.NoError(t, err)
	require.Equal(t, float64(7), res2.Match.Actual.Wickets)

	// Case 3: validation error when neither provided
	_, err = c.historicalMatchBacktest(t.Context(), cutoff, nil, nil)
	require.Error(t, err)
}

// Test GenerateMatch sends cutoff, format, player_ids, features, and match_context and maps response.
func TestBacktestMLClient_GenerateMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/ml/generate-match", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		var payload mlGenerateMatchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.NotEmpty(t, payload.CutoffDate)
		require.Equal(t, []int64{10, 20}, payload.PlayerIDs)
		require.Equal(t, "T20", payload.Format)
		require.Len(t, payload.Features, 2)
		require.Contains(t, payload.Features, "10")
		require.NotNil(t, payload.MatchContext)
		require.Len(t, payload.MatchContext.Team1PlayerIDs, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"players": []map[string]any{
				{"player_id": 10, "runs": 42.0, "wickets": 1.0},
				{"player_id": 20, "runs": 35.0, "wickets": 2.0},
			},
			"innings": []map[string]any{
				{"inning_number": 1, "runs": 170.0, "wickets": 6.0},
				{"inning_number": 2, "runs": 160.0, "wickets": 8.0},
			},
			"win_probability_team1": 0.58,
			"model_version":         "v-test-generate",
		})
	}))
	defer srv.Close()

	t.Setenv("ML_SERVICE_URL", srv.URL)
	c := NewBacktestMLClient()
	cutoff := time.Date(2024, 11, 1, 10, 0, 0, 0, time.UTC)
	features := map[int64]map[string]float64{
		10: {"batting_consistency": 0.5},
		20: {"batting_consistency": 0.7},
	}
	ctx := &MatchContextForReconciliation{
		Team1PlayerIDs:    []int64{10},
		Team2PlayerIDs:    []int64{20},
		VenueID:           1,
		FormatID:          3,
		Team1OppositionID: 100,
		Team2OppositionID: 200,
		Temp:              25,
	}
	res, err := c.GenerateMatch(t.Context(), cutoff, "T20", []int64{10, 20}, features, true, ctx)
	require.NoError(t, err)
	require.Equal(t, "v-test-generate", res.ModelVersion)
	require.Len(t, res.Players, 2)
	require.Len(t, res.Innings, 2)
	require.Greater(t, res.WinProbabilityTeam1, float64(0))
	require.Less(t, res.WinProbabilityTeam1, float64(1))
}
