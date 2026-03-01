package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

// Test PredictPlayers sends cutoff and player_ids and maps response correctly.
func TestBacktestMLClient_PredictPlayers(t *testing.T) {
	// Fake ML server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ml/backtest/predict" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			Cutoff    string  `json:"cutoff_date"`
			PlayerIDs []int64 `json:"player_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		// Validate cutoff and ids propagated
		if payload.Cutoff == "" {
			t.Fatalf("missing cutoff_date")
		}
		expIDs := []int64{1, 2, 3}
		if !reflect.DeepEqual(payload.PlayerIDs, expIDs) {
			t.Fatalf("player_ids = %#v, want %#v", payload.PlayerIDs, expIDs)
		}
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
	if err != nil {
		t.Fatalf("PredictPlayers error: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("len(res) = %d, want 3", len(res))
	}
	if got := res[1]; got.Runs != 10 || got.Wickets != 1 || got.Economy != 6.5 || got.Catches != 2 || got.RunOuts != 1 {
		t.Fatalf("player 1 preds = %+v, want runs=10,wickets=1,economy=6.5,catches=2,run_outs=1", got)
	}
}

// Test PredictMatchAggregates sends cutoff and teams and maps response correctly.
func TestBacktestMLClient_PredictMatchAggregates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ml/backtest/predict" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			Cutoff string    `json:"cutoff_date"`
			Teams  [2]string `json:"teams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload.Cutoff == "" {
			t.Fatalf("missing cutoff_date")
		}
		if payload.Teams != [2]string{"IND", "AUS"} {
			t.Fatalf("teams = %#v, want [IND AUS]", payload.Teams)
		}
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
	if err != nil {
		t.Fatalf("PredictMatchAggregates error: %v", err)
	}
	if agg.Runs != 160 || agg.Wickets != 6 || agg.Extras != 12 || agg.WinnerTeamCode != "IND" {
		t.Fatalf("agg = %+v, want runs=160,wickets=6,extras=12,winner=IND", agg)
	}
}

// Test HistoricalMatchBacktest with match_id selector and with filters selector, plus validation error.
func TestBacktestMLClient_HistoricalMatchBacktest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ml/backtest/match" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
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
	if err != nil {
		t.Fatalf("historicalMatchBacktest by id error: %v", err)
	}
	if res.ModelVersion != "v-test" {
		t.Fatalf("ModelVersion = %q, want v-test", res.ModelVersion)
	}
	if len(res.Players) != 1 {
		t.Fatalf("len(players)=%d, want 1", len(res.Players))
	}
	if res.Match.Predicted.WinnerTeamCode != "IND" || res.Metrics.MAERuns != 2 {
		t.Fatalf("unexpected payload: match=%+v metrics=%+v", res.Match, res.Metrics)
	}

	// Case 2: by filters
	filters := &HistoricalMatchFilters{Format: "T20", Team1: "IND", Team2: "AUS", MatchDate: cutoff}
	res2, err := c.historicalMatchBacktest(t.Context(), cutoff, nil, filters)
	if err != nil {
		t.Fatalf("historicalMatchBacktest by filters error: %v", err)
	}
	if res2.Match.Actual.Wickets != 7 {
		t.Fatalf("expected actual wickets=7, got %+v", res2.Match.Actual)
	}

	// Case 3: validation error when neither provided
	_, err = c.historicalMatchBacktest(t.Context(), cutoff, nil, nil)
	if err == nil {
		t.Fatalf("expected error when neither matchID nor filters provided")
	}
}
