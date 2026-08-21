package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	db "github.com/umayangag/cric-flow/go-app/internal/db"
)

// withBacktestSeams is a small test helper that snapshots all global seam
// function variables used by the backtest accuracy-trend handler and restores
// them automatically via t.Cleanup. Tests provide a setup closure to override
// only the seams they need for their scenario.
func withBacktestSeams(t *testing.T, setup func()) {
	t.Helper()

	// Snapshot originals
	origList := listPlayedMatchesByFilters
	origGetDate := getBacktestMatchDateFunc
	origGetSquad := getBacktestSquadPlayerIDsFunc
	origGetPlayerActs := getBacktestPlayerActualsForMatchFunc
	origMLPlayers := mlBacktestPredictFunc
	origGetAggActs := getBacktestMatchAggregatesActualsFunc
	origMLAgg := mlBacktestPredictMatchAggregatesFunc

	// Restore after test
	t.Cleanup(func() {
		listPlayedMatchesByFilters = origList
		getBacktestMatchDateFunc = origGetDate
		getBacktestSquadPlayerIDsFunc = origGetSquad
		getBacktestPlayerActualsForMatchFunc = origGetPlayerActs
		mlBacktestPredictFunc = origMLPlayers
		getBacktestMatchAggregatesActualsFunc = origGetAggActs
		mlBacktestPredictMatchAggregatesFunc = origMLAgg
	})

	// Allow test to override seams
	if setup != nil {
		setup()
	}
}

// TDD: Happy path for accuracy-trend endpoint with two matches
func TestBacktestAccuracyTrend_HappyPath(t *testing.T) {
	withBacktestSeams(t, func() {
		// Arrange deterministic candidates
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, order string, _ int) ([]backtestCandidate, error) {
			// We ignore ctx type in test; handler passes context.Context, which satisfies interface{}
			m1 := backtestCandidate{
				MatchID:   101,
				MatchDate: time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:    "T20",
				Team1:     "IND",
				Team2:     "AUS",
			}
			m2 := backtestCandidate{
				MatchID:   102,
				MatchDate: time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:    "T20",
				Team1:     "IND",
				Team2:     "AUS",
			}
			if order == "desc" {
				return []backtestCandidate{m2, m1}, nil
			}
			return []backtestCandidate{m1, m2}, nil
		}

		// Cutoff equals the candidate date
		getBacktestMatchDateFunc = func(_ context.Context, matchID int64) (time.Time, error) {
			if matchID == 101 {
				return time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC), nil
			}
			return time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC), nil
		}

		// Same XI for both matches
		getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) {
			return []int64{1, 2, 3}, nil
		}

		// Player actuals: stable across matches
		getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
			return map[int64]playerActuals{1: {Runs: 30}, 2: {Runs: 10}, 3: {Runs: 0}}, nil
		}

		// Player predictions: small errors to produce known MAE
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{
				1: {Runs: 25},
				2: {Runs: 15},
				3: {Runs: 1},
			}, nil // abs: 5,5,1 => MAE=11/3=3.6666
		}

		// Team aggregates actuals and predictions
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, matchID int64) (matchAggregates, error) {
			if matchID == 101 {
				return matchAggregates{Runs: 160, WinnerTeamCode: "IND"}, nil
			}
			return matchAggregates{Runs: 150, WinnerTeamCode: "AUS"}, nil
		}
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
			// Predict constant totals and winner for simplicity
			return matchAggregates{Runs: 155, WinnerTeamCode: "IND"}, "model-v1", nil
		}
	})

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&order=asc",
		nil,
	)

	app.backtestAccuracyTrendHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var payload accuracyTrendResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, 2, payload.Count)
	require.Len(t, payload.Results, 2)
	// player_runs_mae should be ~3.6667 for each
	for i, it := range payload.Results {
		require.InDelta(t, 3.6667, it.Metrics["player_runs_mae"], 0.01, "[%d] player_runs_mae", i)
	}
	// team_runs_mae for m1: |155-160|=5, m2: |155-150|=5, avg=5
	require.InDelta(t, 5.0, payload.Summary["team_runs_mae_avg"], 0.01)
	// winner accuracy: m1 predicted IND vs actual IND => 1, m2 predicted IND vs actual AUS => 0, avg=0.5
	require.InDelta(t, 0.5, payload.Summary["team_winner_accuracy_avg"], 0.01)
	// progressive last should match summary averages approximately
	last := payload.Progressive[len(payload.Progressive)-1]
	require.Equal(t, float64(2), last["n"])
	require.InDelta(t, 0.5, last["team_winner_accuracy_avg"], 0.01)
}

// Verify that order=desc changes the progressive accumulation sequence (while per-match values remain valid)
func TestBacktestAccuracyTrend_OrderingDesc_Progressive(t *testing.T) {
	withBacktestSeams(t, func() {
		// Arrange candidates in desc order based on query
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, order string, _ int) ([]backtestCandidate, error) {
			m1 := backtestCandidate{
				MatchID:   201,
				MatchDate: time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:    "T20",
				Team1:     "IND",
				Team2:     "AUS",
			}
			m2 := backtestCandidate{
				MatchID:   202,
				MatchDate: time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:    "T20",
				Team1:     "IND",
				Team2:     "AUS",
			}
			if order == "desc" {
				return []backtestCandidate{m2, m1}, nil
			}
			return []backtestCandidate{m1, m2}, nil
		}

		// Cutoff equals candidate date
		getBacktestMatchDateFunc = func(_ context.Context, matchID int64) (time.Time, error) {
			if matchID == 201 {
				return time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC), nil
			}
			return time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC), nil
		}
		getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1, 2, 3}, nil }
		getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
			return map[int64]playerActuals{1: {Runs: 30}, 2: {Runs: 10}, 3: {Runs: 0}}, nil
		}
		// Predictions produce constant MAE and team metrics
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 25}, 2: {Runs: 15}, 3: {Runs: 1}}, nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, matchID int64) (matchAggregates, error) {
			if matchID == 201 {
				return matchAggregates{Runs: 160, WinnerTeamCode: "IND"}, nil
			}
			return matchAggregates{Runs: 150, WinnerTeamCode: "AUS"}, nil
		}
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
			return matchAggregates{Runs: 155, WinnerTeamCode: "IND"}, "model-v1", nil
		}
	})

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&order=desc",
		nil,
	)

	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var payload accuracyTrendResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, 2, payload.Count)
	// Progressive first item reflects first result (desc: matchID 202)
	require.Len(t, payload.Progressive, 2)
	require.Equal(t, float64(1), payload.Progressive[0]["n"])
	// Winner accuracy average after first item must be either 1 or 0; last should be 0.5 as in happy path
	last := payload.Progressive[1]
	require.Equal(t, float64(2), last["n"])
	require.InDelta(t, 0.5, last["team_winner_accuracy_avg"], 0.01)
}

// Verify that limit parameter reduces the candidate set used for metrics
func TestBacktestAccuracyTrend_Limit(t *testing.T) {
	withBacktestSeams(t, func() {
		// Return three candidates but honor the limit argument
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, order string, limit int) ([]backtestCandidate, error) {
			m1 := backtestCandidate{
				MatchID:   301,
				MatchDate: time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:    "T20",
				Team1:     "IND",
				Team2:     "AUS",
			}
			m2 := backtestCandidate{
				MatchID:   302,
				MatchDate: time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:    "T20",
				Team1:     "IND",
				Team2:     "AUS",
			}
			m3 := backtestCandidate{
				MatchID:   303,
				MatchDate: time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:    "T20",
				Team1:     "IND",
				Team2:     "AUS",
			}
			all := []backtestCandidate{m1, m2, m3}
			if order == "desc" {
				all = []backtestCandidate{m3, m2, m1}
			}
			if limit > 0 && limit < len(all) {
				return all[:limit], nil
			}
			return all, nil
		}

		getBacktestMatchDateFunc = func(_ context.Context, matchID int64) (time.Time, error) {
			switch matchID {
			case 301:
				return time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC), nil
			case 302:
				return time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC), nil
			default:
				return time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC), nil
			}
		}
		getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1, 2, 3}, nil }
		getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
			return map[int64]playerActuals{1: {Runs: 30}, 2: {Runs: 10}, 3: {Runs: 0}}, nil
		}
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 25}, 2: {Runs: 15}, 3: {Runs: 1}}, nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, matchID int64) (matchAggregates, error) {
			// Alternate winners to avoid degenerate averages
			switch matchID {
			case 301:
				return matchAggregates{Runs: 160, WinnerTeamCode: "IND"}, nil
			case 302:
				return matchAggregates{Runs: 150, WinnerTeamCode: "AUS"}, nil
			default:
				return matchAggregates{Runs: 140, WinnerTeamCode: "IND"}, nil
			}
		}
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
			return matchAggregates{Runs: 155, WinnerTeamCode: "IND"}, "model-v1", nil
		}
	})

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&order=asc&limit=2",
		nil,
	)

	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var payload accuracyTrendResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, 2, payload.Count)
	require.Len(t, payload.Results, 2)
}

// Verify that start_date/end_date filters are parsed and passed through to the seam
func TestBacktestAccuracyTrend_DateRangeFiltering(t *testing.T) {
	withBacktestSeams(t, func() {
		// Capture received start/end and return candidates accordingly
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, start, end time.Time, order string, _ int) ([]backtestCandidate, error) {
			// Expect start=2024-10-11 and end=2024-10-25
			require.False(t, start.IsZero(), "expected non-zero start date")
			require.False(t, end.IsZero(), "expected non-zero end date")
			// Build three dates; only middle one within range
			d1 := time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC)
			d2 := time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC)
			d3 := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)
			all := []backtestCandidate{
				{MatchID: 401, MatchDate: d1.Format(time.RFC3339), Format: "T20", Team1: "IND", Team2: "AUS"},
				{MatchID: 402, MatchDate: d2.Format(time.RFC3339), Format: "T20", Team1: "IND", Team2: "AUS"},
				{MatchID: 403, MatchDate: d3.Format(time.RFC3339), Format: "T20", Team1: "IND", Team2: "AUS"},
			}
			// Simulate repo applying date filter
			filtered := make([]backtestCandidate, 0, 1)
			for _, c := range all {
				cd, _ := time.Parse(time.RFC3339, c.MatchDate)
				if (cd.Equal(start) || cd.After(start)) && (cd.Equal(end) || cd.Before(end)) {
					filtered = append(filtered, c)
				}
			}
			if order == "desc" {
				// reverse
				for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
					filtered[i], filtered[j] = filtered[j], filtered[i]
				}
			}
			return filtered, nil
		}

		// Stub minimal other seams to avoid nil pointer in handler metric loop
		getBacktestMatchDateFunc = func(_ context.Context, matchID int64) (time.Time, error) {
			switch matchID {
			case 402:
				return time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC), nil
			default:
				return time.Time{}, nil
			}
		}
		getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1}, nil }
		getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
			return map[int64]playerActuals{1: {Runs: 10}}, nil
		}
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 12}}, nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
			return matchAggregates{Runs: 150, WinnerTeamCode: "IND"}, nil
		}
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
			return matchAggregates{Runs: 152, WinnerTeamCode: "IND"}, "model-v1", nil
		}
	})

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&start_date=2024-10-11&end_date=2024-10-25",
		nil,
	)

	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var payload accuracyTrendResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, 1, payload.Count)
	require.Len(t, payload.Results, 1)
	require.Equal(t, int64(402), payload.Results[0].MatchID)
}

// Verify that team1/team2 filters are passed; seam returns only when both match
func TestBacktestAccuracyTrend_TeamFiltering(t *testing.T) {
	withBacktestSeams(t, func() {
		listPlayedMatchesByFilters = func(_ context.Context, _ string, team1, team2 string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
			// Only return if IND vs AUS requested
			if team1 == "IND" && team2 == "AUS" {
				return []backtestCandidate{
					{
						MatchID:   501,
						MatchDate: time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
						Format:    "T20",
						Team1:     "IND",
						Team2:     "AUS",
					},
				}, nil
			}
			return []backtestCandidate{}, nil
		}

		// Minimal stubs for metrics
		getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
			return time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC), nil
		}
		getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1}, nil }
		getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
			return map[int64]playerActuals{1: {Runs: 10}}, nil
		}
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 10}}, nil
		}
	})

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS", nil)
	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var payload accuracyTrendResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, 1, payload.Count)
	require.Equal(t, "IND", payload.Results[0].Team1)
	require.Equal(t, "AUS", payload.Results[0].Team2)
}

// --- Cache mode tests ---

// cache=read should use cached aggregates if available and must not call ML match aggregates seam
func TestBacktestAccuracyTrend_CacheRead_UsesCache(t *testing.T) {
	withBacktestSeams(t, func() {
		// One candidate
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
			return []backtestCandidate{{
				MatchID:   601,
				MatchDate: time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:    "T20",
				Team1:     "IND",
				Team2:     "AUS",
			}}, nil
		}

		// Cutoff
		getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
			return time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC), nil
		}

		// Minimal player seams to enable player_runs_mae
		getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1}, nil }
		getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
			return map[int64]playerActuals{1: {Runs: 10}}, nil
		}
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 11}}, nil
		}

		// Cached record present
		getMatchPredictionAggregatesFunc = func(_ context.Context, matchID int64) (dbRec db.MatchPredictionAggregates, err error) {
			return db.MatchPredictionAggregates{
				MatchID:             matchID,
				Format:              "T20",
				Team1Code:           "IND",
				Team2Code:           "AUS",
				PredictedWinnerCode: sql.NullString{String: "IND", Valid: true},
				PredictedTotalRuns:  sql.NullFloat64{Float64: 155, Valid: true},
				CutoffAt:            time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC),
			}, nil
		}

		// Make ML match aggregates seam fail if called (should not be when cache=read)
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
			require.Fail(t, "ML match aggregates was called despite cache=read")
			return matchAggregates{}, "", nil
		}

		// Actuals for aggregates
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
			return matchAggregates{Runs: 150, WinnerTeamCode: "IND"}, nil
		}
	})

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&cache=read",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var payload accuracyTrendResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, 1, payload.Count)
	// team_runs_mae should be |155-150| = 5 from cached predictions
	require.InDelta(t, 5.0, payload.Results[0].Metrics["team_runs_mae"], 0.01)
}

// cache=off should ignore cache even if present and use ML predictions
func TestBacktestAccuracyTrend_CacheOff_IgnoresCache(t *testing.T) {
	withBacktestSeams(t, func() {
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
			return []backtestCandidate{
				{
					MatchID:   602,
					MatchDate: time.Date(2024, 10, 11, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
					Format:    "T20",
					Team1:     "IND",
					Team2:     "AUS",
				},
			}, nil
		}
		getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
			return time.Date(2024, 10, 11, 14, 0, 0, 0, time.UTC), nil
		}
		getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1}, nil }
		getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
			return map[int64]playerActuals{1: {Runs: 10}}, nil
		}
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 9}}, nil
		}

		// Cache present but should be ignored
		getMatchPredictionAggregatesFunc = func(_ context.Context, _ int64) (db.MatchPredictionAggregates, error) {
			return db.MatchPredictionAggregates{
				MatchID:             602,
				Format:              "T20",
				Team1Code:           "IND",
				Team2Code:           "AUS",
				PredictedWinnerCode: sql.NullString{String: "AUS", Valid: true},
				PredictedTotalRuns:  sql.NullFloat64{Float64: 140, Valid: true},
				CutoffAt:            time.Date(2024, 10, 11, 14, 0, 0, 0, time.UTC),
			}, nil
		}
		// ML returns different value to detect path: predicted 152 → MAE |152-150|=2
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
			return matchAggregates{Runs: 152, WinnerTeamCode: "IND"}, "test-model", nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
			return matchAggregates{Runs: 150, WinnerTeamCode: "IND"}, nil
		}
	})

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&cache=off",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var payload accuracyTrendResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, 1, payload.Count)
	// Expect ML value 152 vs actual 150 => MAE 2 (not using cached 140)
	require.InDelta(t, 2.0, payload.Results[0].Metrics["team_runs_mae"], 0.01)
}

// cache=readwrite should compute on miss and upsert cache
func TestBacktestAccuracyTrend_CacheReadWrite_UpsertsOnMiss(t *testing.T) {
	// Track whether upsert is called on cache miss
	called := false
	withBacktestSeams(t, func() {
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
			return []backtestCandidate{
				{
					MatchID:   603,
					MatchDate: time.Date(2024, 10, 12, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
					Format:    "T20",
					Team1:     "IND",
					Team2:     "AUS",
				},
			}, nil
		}
		getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
			return time.Date(2024, 10, 12, 14, 0, 0, 0, time.UTC), nil
		}
		getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) { return []int64{1}, nil }
		getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
			return map[int64]playerActuals{1: {Runs: 10}}, nil
		}
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64, _ bool, _ *MatchContextForReconciliation) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 10}}, nil
		}

		// Cache miss
		getMatchPredictionAggregatesFunc = func(_ context.Context, _ int64) (db.MatchPredictionAggregates, error) {
			return db.MatchPredictionAggregates{}, sql.ErrNoRows
		}
		// ML compute path
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
			return matchAggregates{Runs: 149, WinnerTeamCode: "IND"}, "test-model", nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
			return matchAggregates{Runs: 150, WinnerTeamCode: "IND"}, nil
		}

		// Capture upsert invocation
		upsertMatchPredictionAggregatesFunc = func(_ context.Context, row db.MatchPredictionAggregates) error {
			called = true
			require.Equal(t, int64(603), row.MatchID)
			require.Equal(t, "IND", row.Team1Code)
			require.Equal(t, "AUS", row.Team2Code)
			require.True(t, row.PredictedTotalRuns.Valid)
			require.Equal(t, float64(149), row.PredictedTotalRuns.Float64)
			return nil
		}

		// Ensure subsequent read would find cache (simulate by overriding get to return same record after upsert)
		// Not strictly necessary for single-call verification.
	})

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&cache=readwrite",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	require.True(t, called, "expected upsertMatchPredictionAggregatesFunc to be called")
	require.Equal(t, http.StatusOK, rr.Code)
}
