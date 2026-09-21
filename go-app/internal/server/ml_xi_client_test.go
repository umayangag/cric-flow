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

func TestDateParam_RendersADateAndOmitsTheZeroTime(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", dateParam(time.Time{}))
	assert.Equal(t, "2025-09-01", dateParam(time.Date(2025, 9, 1, 14, 30, 0, 0, time.UTC)))
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
			client, captured := xiCaptureServer(t, `{"team1_win_probability":0.6,"objective_probability":0.55,`+
				`"served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`)

			win, err := client.PredictMatchWinXI(context.Background(), predictteam.XIWinRequest{
				Format:          "T20",
				Team1PlayerKeys: []string{"a1"},
				Team2PlayerKeys: []string{"b1"},
				AsOf:            tc.asOf,
			})

			require.NoError(t, err)
			assert.InDelta(t, 0.6, win.Team1WinProbability, 1e-9)
			assert.Equal(t, servedFromTheDevRun, win.Served, "the answer names the state it was read from")
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
		t, `{"selected_player_ids":["a1"],"win_probability":0.5,"evaluations":1,"improved_over_seed":0,`+
			`"served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`,
	)

	result, err := client.OptimizeXI(context.Background(), predictteam.XIOptimizationRequest{
		Format:             "T20",
		PoolPlayerKeys:     []string{"a1", "a2"},
		OpponentPlayerKeys: []string{"b1"},
		AsOf:               time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
	})

	require.NoError(t, err)
	assert.Equal(t, "2025-03-01", (*captured)["as_of"])
	assert.Equal(t, servedFromTheDevRun, result.Served)
}

// The "why this player" state crosses the boundary as a registry id plus numbers; go-app
// resolves the id against the pool, so the client's whole job is to carry it intact (P1-3).
func TestOptimizeXI_MapsTheSelectionReasonsIncludingTheAlternativesRegistryID(t *testing.T) {
	t.Parallel()

	client, _ := xiCaptureServer(
		t, `{"selected_player_ids":["a1"],"win_probability":0.5,"evaluations":1,"improved_over_seed":0,`+
			`"selection_reasons":{"a1":{"roles":["keeper","bowling_option"],"selection_rating":1.25,`+
			`"rating_percentile":92.5,"pool_size":24,`+
			`"best_alternative":{"player_id":"a9","win_probability_gap":0.0131},`+
			`"best_alternative_note":null}},`+
			`"served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`,
	)

	result, err := client.OptimizeXI(context.Background(), predictteam.XIOptimizationRequest{
		Format:             "T20I",
		PoolPlayerKeys:     []string{"a1", "a9"},
		OpponentPlayerKeys: []string{"b1"},
	})

	require.NoError(t, err)
	require.Len(t, result.SelectionReasons, 1)
	reason := result.SelectionReasons["a1"]
	assert.Equal(t, []string{predictteam.RoleKeeper, predictteam.RoleBowlingOption}, reason.Roles)
	assert.InDelta(t, 1.25, reason.SelectionRating, 1e-9)
	assert.InDelta(t, 92.5, reason.RatingPercentile, 1e-9)
	assert.Equal(t, 24, reason.PoolSize)
	assert.Equal(t, "a9", reason.BestAlternativeKey)
	assert.InDelta(t, 0.0131, reason.BestAlternativeGap, 1e-9)
	assert.Empty(t, reason.BestAlternativeNote)
}

// A rating-ordered answer maximised nothing, so it names no alternative and go-app must
// not manufacture one.
func TestOptimizeXI_KeepsARatingOrderedReasonFreeOfAnAlternative(t *testing.T) {
	t.Parallel()

	client, _ := xiCaptureServer(
		t, `{"selected_player_ids":["a1"],"objective":"ratings","optimised":false,"evaluations":0,`+
			`"improved_over_seed":0,`+
			`"selection_reasons":{"a1":{"roles":[],"selection_rating":0.4,"rating_percentile":10,`+
			`"pool_size":18,"best_alternative":null,"best_alternative_note":null}},`+
			`"served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`,
	)

	result, err := client.OptimizeXI(context.Background(), predictteam.XIOptimizationRequest{
		Format:         "TEST",
		Objective:      predictteam.SelectionObjectiveRatings,
		PoolPlayerKeys: []string{"a1", "a9"},
	})

	require.NoError(t, err)
	assert.Empty(t, result.SelectionReasons["a1"].BestAlternativeKey)
	assert.Empty(t, result.SelectionReasons["a1"].BestAlternativeNote)
	assert.Equal(t, 18, result.SelectionReasons["a1"].PoolSize)
}

// servedFromTheDevRun is the stamp the wire fixtures in this file carry: the run the dev
// stack was serving when P1-5 shipped, with its ratings through the date they ran through.
var servedFromTheDevRun = predictteam.ServedRatings{
	RunID:          "20260906T083819Z-36689f80",
	RatingsThrough: "2026-09-02",
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
	  "unknown_player_ids": [],
	  "served_ratings": {"run_id": "20260906T083819Z-36689f80", "ratings_through": "2026-09-02"}
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
	assert.Equal(t, servedFromTheDevRun, result.Served)
}

// SERVE-04: the fixture's own date reaches ml-service.
//
// Until this, no request model carried one, so ml-service dated every fixture by the last
// match in its own rating state — twelve days behind today on the dev box, and further for
// any fixture worth asking about. go-app has always held the date the caller typed
// (`Input.MatchDate`); it simply never sent it. Both payloads carry it, because the rows
// behind a simulation and behind a performance prediction are the same rows and must be
// dated identically.
func TestPerformanceAndSimulatePayloads_CarryTheFixtureDate(t *testing.T) {
	t.Parallel()

	matchDate := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	simulateResponse := `{"format":"T20","n_samples":300,"seed":0,"toss_marginalised":true,
	  "team1":{"total":{"q10":130,"median":158,"q90":186,"mean":158.4,"sd":21.0,"scorecard":157.9},
	           "extras_scorecard":7.5,"extras_spread_share":0.02,
	           "wickets_lost":{"q10":3,"median":6,"q90":9},"players":[]},
	  "team2":{"total":{"q10":120,"median":150,"q90":180,"mean":150.1,"sd":22.0,"scorecard":150.2},
	           "extras_scorecard":7.1,"extras_spread_share":0.02,
	           "wickets_lost":{"q10":3,"median":6,"q90":9},"players":[]},
	  "win_probability":{"simulated":0.57,"p_tie":0.01,"display":0.6,"headline":0.6,"headline_source":"display"},
	  "margin":{"p_bat_first_wins":0.5,"p_chaser_wins":0.49,"p_tie":0.01},
	  "unknown_player_ids":[],
	  "served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`
	performanceResponse := `{"players":[],"innings_marginalised":true,
	  "venue_context":{"venue_bf_rate":0.5,"venue_n":0,"neutral":true},
	  "served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`

	testCases := []struct {
		name      string
		response  string
		call      func(*MLClient, time.Time) error
		wantField bool
		wantValue string
	}{
		{
			name:     "a simulated fixture is dated by the caller",
			response: simulateResponse,
			call: func(client *MLClient, day time.Time) error {
				_, err := client.SimulateMatchXI(context.Background(), predictteam.XISimulationRequest{
					Format:          "T20",
					Team1PlayerKeys: []string{"a1"},
					Team2PlayerKeys: []string{"b1"},
					MatchDate:       day,
					Samples:         300,
				})
				return err
			},
			wantField: true,
			wantValue: "2026-10-21",
		},
		{
			name:     "a performance prediction is dated by the caller",
			response: performanceResponse,
			call: func(client *MLClient, day time.Time) error {
				_, err := client.PredictPerformance(context.Background(), predictteam.XIPerformanceRequest{
					Format:          "T20",
					Team1PlayerKeys: []string{"a1"},
					Team2PlayerKeys: []string{"b1"},
					MatchDate:       day,
				})
				return err
			},
			wantField: true,
			wantValue: "2026-10-21",
		},
		{
			name:     "no date is no field, and ml-service says it dated the fixture itself",
			response: performanceResponse,
			call: func(client *MLClient, _ time.Time) error {
				_, err := client.PredictPerformance(context.Background(), predictteam.XIPerformanceRequest{
					Format:          "T20",
					Team1PlayerKeys: []string{"a1"},
					Team2PlayerKeys: []string{"b1"},
				})
				return err
			},
			wantField: false,
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, captured := xiCaptureServer(t, tc.response)

			err := tc.call(client, matchDate)

			require.NoError(t, err)
			value, present := (*captured)["match_date"]
			assert.Equal(t, tc.wantField, present)
			if tc.wantField {
				assert.Equal(t, tc.wantValue, value)
			}
		})
	}
}

// Play mode's constraint check rides on the call that scores the eleven (P1-2): the
// request carries the constraints only where the caller pinned an eleven, and the check
// that comes back describes that same eleven.
func TestPredictMatchWinXI_SendsConstraintsOnlyForAPinnedElevenAndMapsTheCheck(t *testing.T) {
	t.Parallel()
	client, captured := xiCaptureServer(t, `{"team1_win_probability":0.62,"objective_probability":0.55,
	  "team1_constraint_check":{"team_size":11,"bowlers":4,"min_bowlers":5,"has_keeper":true,
	                            "require_keeper":true,"missing_must_include":["a9"],"met":false},
	  "team2_constraint_check":{"team_size":11,"bowlers":6,"min_bowlers":5,"has_keeper":true,
	                            "require_keeper":true,"missing_must_include":[],"met":true},
	  "served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`)

	win, err := client.PredictMatchWinXI(context.Background(), predictteam.XIWinRequest{
		Format:          "T20I",
		Team1PlayerKeys: []string{"a1"},
		Team2PlayerKeys: []string{"b1"},
		Team1Constraints: &predictteam.ConstraintCheckRequest{
			Constraints:     predictteam.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true},
			MustIncludeKeys: []string{"a9"},
		},
		Team2Constraints: &predictteam.ConstraintCheckRequest{
			Constraints: predictteam.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true},
		},
	})

	require.NoError(t, err)
	sent, present := (*captured)["team1_constraints"].(map[string]interface{})
	require.True(t, present, "a pinned eleven is checked against the constraints it was sent with")
	assert.Equal(t, float64(5), sent["min_bowlers"])
	assert.Equal(t, []interface{}{"a9"}, sent["must_include"])
	require.NotNil(t, win.Team1Check)
	assert.False(t, win.Team1Check.Met)
	assert.Equal(t, 4, win.Team1Check.Bowlers)
	assert.Equal(t, []string{"a9"}, win.Team1Check.MissingMustIncludeKeys)
	require.NotNil(t, win.Team2Check)
	assert.True(t, win.Team2Check.Met)
}

func TestPredictMatchWinXI_SendsNoConstraintsForASearchedEleven(t *testing.T) {
	t.Parallel()
	client, captured := xiCaptureServer(t, `{"team1_win_probability":0.6,"objective_probability":0.55,
	  "served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`)

	win, err := client.PredictMatchWinXI(context.Background(), predictteam.XIWinRequest{
		Format:          "T20I",
		Team1PlayerKeys: []string{"a1"},
		Team2PlayerKeys: []string{"b1"},
	})

	require.NoError(t, err)
	assert.NotContains(t, *captured, "team1_constraints", "the optimiser applied them while it searched")
	assert.NotContains(t, *captured, "team2_constraints")
	assert.Nil(t, win.Team1Check, "no check was asked for, and none is invented")
	assert.Nil(t, win.Team2Check)
}

// A transport failure is a named refusal on the wire, not a 500 with a dial string for a
// message: the code, the endpoint and the reason survive to the response (P1-4, §8.7).
func TestPredictMatchWinXI_NamesAnUnreachableServiceOnTheWire(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	client := &MLClient{BaseURL: srv.URL, HTTP: &http.Client{Timeout: time.Second}}

	_, err := client.PredictMatchWinXI(context.Background(), predictteam.XIWinRequest{
		Format:          "T20I",
		Team1PlayerKeys: []string{"a1"},
		Team2PlayerKeys: []string{"b1"},
	})

	var mlErr *mlServiceError
	require.ErrorAs(t, err, &mlErr)
	assert.Equal(t, mlUnreachableCode, mlErr.Code)
	assert.Equal(t, http.StatusBadGateway, mlErr.Status)
	assert.Equal(t, "/xi/predict-win", mlErr.Endpoint)
	assert.Contains(t, mlErr.Message, "did not answer /xi/predict-win")
	assert.Contains(t, mlErr.Hint, "ML_SERVICE_URL")
}

// TestOptimizeXI_SendsTheMustIncludeLock is the wire half of B-10: the request's
// must-include ids reach `/xi/optimize` as `must_include`, where they were sent as an
// empty list whatever the request carried, so a "must include" bound nothing.
func TestOptimizeXI_SendsTheMustIncludeLock(t *testing.T) {
	t.Parallel()

	client, captured := xiCaptureServer(
		t, `{"selected_player_ids":["a1","a9"],"win_probability":0.5,"evaluations":1,"improved_over_seed":0,`+
			`"served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`,
	)

	_, err := client.OptimizeXI(context.Background(), predictteam.XIOptimizationRequest{
		Format:             "T20I",
		PoolPlayerKeys:     []string{"a1", "a9"},
		OpponentPlayerKeys: []string{"b1"},
		MustIncludeKeys:    []string{"a9"},
	})

	require.NoError(t, err)
	sent, present := (*captured)["constraints"].(map[string]interface{})
	require.True(t, present)
	assert.Equal(t, []interface{}{"a9"}, sent["must_include"])
}

// TestOptimizeXI_WithNoMustIncludeSendsAnEmptyLock holds the default still: a request that
// requires nobody sends `must_include: []`, the payload every call sent before B-10.
func TestOptimizeXI_WithNoMustIncludeSendsAnEmptyLock(t *testing.T) {
	t.Parallel()

	client, captured := xiCaptureServer(
		t, `{"selected_player_ids":["a1"],"win_probability":0.5,"evaluations":1,"improved_over_seed":0,`+
			`"served_ratings":{"run_id":"20260906T083819Z-36689f80","ratings_through":"2026-09-02"}}`,
	)

	_, err := client.OptimizeXI(context.Background(), predictteam.XIOptimizationRequest{
		Format:             "T20I",
		PoolPlayerKeys:     []string{"a1", "a2"},
		OpponentPlayerKeys: []string{"b1"},
	})

	require.NoError(t, err)
	sent, present := (*captured)["constraints"].(map[string]interface{})
	require.True(t, present)
	assert.Equal(t, []interface{}{}, sent["must_include"])
}
