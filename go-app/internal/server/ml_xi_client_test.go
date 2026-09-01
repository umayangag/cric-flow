package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

func TestAsOfParam_RendersADateAndOmitsTheZeroTime(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", asOfParam(time.Time{}))
	assert.Equal(t, "2025-09-01", asOfParam(time.Date(2025, 9, 1, 14, 30, 0, 0, time.UTC)))
}

// xiCaptureServer answers any /xi/* POST with the given body and records the request JSON.
func xiCaptureServer(t *testing.T, response string) (*MLClient, *map[string]interface{}) {
	t.Helper()
	captured := map[string]interface{}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &captured))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return &MLClient{BaseURL: srv.URL, HTTP: srv.Client()}, &captured
}

func TestPredictMatchWinXI_SendsAsOfOnlyWhenSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		asOf      time.Time
		wantField bool
		wantValue string
	}{
		{
			name:      "a backtest date is sent as YYYY-MM-DD",
			asOf:      time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			wantField: true,
			wantValue: "2025-03-01",
		},
		{name: "a live prediction omits the field", asOf: time.Time{}, wantField: false},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, captured := xiCaptureServer(t, `{"team1_win_probability":0.6,"objective_probability":0.55}`)

			p, err := client.PredictMatchWinXI(context.Background(), predictteam.XIWinRequest{
				Format:          "T20",
				Team1PlayerKeys: []string{"a1"},
				Team2PlayerKeys: []string{"b1"},
				AsOf:            tc.asOf,
			})

			require.NoError(t, err)
			assert.InDelta(t, 0.6, p, 1e-9)
			value, present := (*captured)["as_of"]
			assert.Equal(t, tc.wantField, present)
			if tc.wantField {
				assert.Equal(t, tc.wantValue, value)
			}
		})
	}
}

func TestOptimizeXI_SendsAsOf(t *testing.T) {
	t.Parallel()

	client, captured := xiCaptureServer(
		t, `{"selected_player_ids":["a1"],"win_probability":0.5,"evaluations":1,"improved_over_seed":0}`,
	)

	_, err := client.OptimizeXI(context.Background(), predictteam.XIOptimizationRequest{
		Format:             "T20",
		PoolPlayerKeys:     []string{"a1", "a2"},
		OpponentPlayerKeys: []string{"b1"},
		AsOf:               time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
	})

	require.NoError(t, err)
	assert.Equal(t, "2025-03-01", (*captured)["as_of"])
}

func TestSimulateMatchXI_MapsTheResponseAndSendsTheFixture(t *testing.T) {
	t.Parallel()
	response := `{
	  "format": "T20", "n_samples": 2000, "seed": 0, "toss_marginalised": true,
	  "team1": {"total": {"q10": 130, "median": 158, "q90": 186, "mean": 158.4, "sd": 21.0, "scorecard": 157.9},
	            "extras_scorecard": 7.5, "extras_spread_share": 0.02, "wickets_lost": {"q10": 3, "median": 6, "q90": 9},
	            "players": [{"player_id": "a1", "side": 1, "p_bats": 1.0, "p_bowls": 0.0,
	                         "runs": {"q10": 5, "median": 28, "q90": 61}, "balls_faced": {"q10": 4, "median": 22, "q90": 44},
	                         "wickets": {"q10": 0, "median": 0, "q90": 0}, "runs_conceded": {"q10": 0, "median": 0, "q90": 0},
	                         "balls_bowled": {"q10": 0, "median": 0, "q90": 0},
	                         "scorecard": {"runs": 29.1, "balls_faced": 22.3, "wickets": 0, "runs_conceded": 0, "balls_bowled": 0},
	                         "spread_share": 0.18, "spread_runs": 3.8}]},
	  "team2": {"total": {"q10": 120, "median": 150, "q90": 180, "mean": 150.1, "sd": 22.0, "scorecard": 150.2},
	            "extras_scorecard": 7.1, "extras_spread_share": 0.02, "wickets_lost": {"q10": 3, "median": 6, "q90": 9}, "players": []},
	  "win_probability": {"simulated": 0.57, "p_tie": 0.01, "display": 0.6, "headline": 0.6, "headline_source": "display"},
	  "margin": {"p_bat_first_wins": 0.5, "p_chaser_wins": 0.49, "p_tie": 0.01},
	  "unknown_player_ids": []
	}`
	client, captured := xiCaptureServer(t, response)
	batsFirst := true

	result, err := client.SimulateMatchXI(context.Background(), predictteam.XISimulationRequest{
		Format:          "T20",
		Team1PlayerKeys: []string{"a1"},
		Team2PlayerKeys: []string{"b1"},
		Team1ID:         7,
		VenueID:         3,
		Team1BatsFirst:  &batsFirst,
		AsOf:            time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		Samples:         500,
	})

	require.NoError(t, err)
	assert.Equal(t, "2025-03-01", (*captured)["as_of"])
	assert.Equal(t, float64(500), (*captured)["n_samples"])
	assert.Equal(t, true, (*captured)["team1_bats_first"])
	assert.Equal(t, float64(7), (*captured)["team1_id"])
	_, hasTeam2 := (*captured)["team2_id"]
	assert.False(t, hasTeam2, "an unknown team id is omitted")
	assert.Equal(t, 2000, result.Samples)
	assert.True(t, result.TossMarginalised)
	assert.InDelta(t, 157.9, result.Team1.TotalScorecard, 1e-9)
	assert.InDelta(t, 130, result.Team1.Total.P10, 1e-9)
	require.Len(t, result.Team1.Players, 1)
	assert.Equal(t, "a1", result.Team1.Players[0].PlayerKey)
	assert.InDelta(t, 29.1, result.Team1.Players[0].ScorecardRuns, 1e-9)
	assert.InDelta(t, 61, result.Team1.Players[0].Runs.P90, 1e-9)
	assert.InDelta(t, 0.18, result.Team1.Players[0].SpreadShare, 1e-9)
	assert.InDelta(t, 0.57, result.SimulatedTeam1WinProbability, 1e-9)
	assert.InDelta(t, 0.6, result.HeadlineTeam1WinProbability, 1e-9)
	assert.Equal(t, "display", result.HeadlineSource)
}
