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
	res, err := c.predictPlayers(t.Context(), cutoff, []int64{1, 2, 3})
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
