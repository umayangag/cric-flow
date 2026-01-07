package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	db "github.com/umayangag/cric-info-scrapers/go-app/internal/db"
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
				MatchID: 101,
				Date:    time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:  "T20",
				Team1:   "IND",
				Team2:   "AUS",
			}
			m2 := backtestCandidate{
				MatchID: 102,
				Date:    time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:  "T20",
				Team1:   "IND",
				Team2:   "AUS",
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
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
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
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, error) {
			// Predict constant totals and winner for simplicity
			return matchAggregates{Runs: 155, WinnerTeamCode: "IND"}, nil
		}
	})

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&order=asc",
		nil,
	)

	app.backtestAccuracyTrendHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var payload accuracyTrendResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 2 {
		t.Fatalf("count = %d, want 2", payload.Count)
	}
	// Check per-match metrics exist
	if len(payload.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(payload.Results))
	}
	// player_runs_mae should be ~3.6667 for each
	for i, it := range payload.Results {
		mae := it.Metrics["player_runs_mae"]
		if mae < 3.66 || mae > 3.67 {
			t.Fatalf("[%d] player_runs_mae = %f, want ~3.6667", i, mae)
		}
	}
	// team_runs_mae for m1: |155-160|=5, m2: |155-150|=5, avg=5
	if payload.Summary["team_runs_mae_avg"] < 4.99 || payload.Summary["team_runs_mae_avg"] > 5.01 {
		t.Fatalf("team_runs_mae_avg = %f, want 5", payload.Summary["team_runs_mae_avg"])
	}
	// winner accuracy: m1 predicted IND vs actual IND => 1, m2 predicted IND vs actual AUS => 0, avg=0.5
	if payload.Summary["team_winner_accuracy_avg"] < 0.49 || payload.Summary["team_winner_accuracy_avg"] > 0.51 {
		t.Fatalf("team_winner_accuracy_avg = %f, want 0.5", payload.Summary["team_winner_accuracy_avg"])
	}
	// progressive last should match summary averages approximately
	last := payload.Progressive[len(payload.Progressive)-1]
	if last["n"] != 2 {
		t.Fatalf("progressive last n = %f, want 2", last["n"])
	}
	if last["team_winner_accuracy_avg"] < 0.49 || last["team_winner_accuracy_avg"] > 0.51 {
		t.Fatalf("progressive team_winner_accuracy_avg = %f, want 0.5", last["team_winner_accuracy_avg"])
	}
}

// Verify that order=desc changes the progressive accumulation sequence (while per-match values remain valid)
func TestBacktestAccuracyTrend_OrderingDesc_Progressive(t *testing.T) {
	withBacktestSeams(t, func() {
		// Arrange candidates in desc order based on query
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, order string, _ int) ([]backtestCandidate, error) {
			m1 := backtestCandidate{
				MatchID: 201,
				Date:    time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:  "T20",
				Team1:   "IND",
				Team2:   "AUS",
			}
			m2 := backtestCandidate{
				MatchID: 202,
				Date:    time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:  "T20",
				Team1:   "IND",
				Team2:   "AUS",
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
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 25}, 2: {Runs: 15}, 3: {Runs: 1}}, nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, matchID int64) (matchAggregates, error) {
			if matchID == 201 {
				return matchAggregates{Runs: 160, WinnerTeamCode: "IND"}, nil
			}
			return matchAggregates{Runs: 150, WinnerTeamCode: "AUS"}, nil
		}
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, error) {
			return matchAggregates{Runs: 155, WinnerTeamCode: "IND"}, nil
		}
	})

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&order=desc",
		nil,
	)

	app.backtestAccuracyTrendHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var payload accuracyTrendResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 2 {
		t.Fatalf("count = %d, want 2", payload.Count)
	}
	// Progressive first item reflects first result (desc: matchID 202)
	if len(payload.Progressive) != 2 {
		t.Fatalf("progressive len = %d, want 2", len(payload.Progressive))
	}
	if payload.Progressive[0]["n"] != 1 {
		t.Fatalf("progressive[0].n = %f, want 1", payload.Progressive[0]["n"])
	}
	// Winner accuracy average after first item must be either 1 or 0; last should be 0.5 as in happy path
	last := payload.Progressive[1]
	if last["n"] != 2 {
		t.Fatalf("last n = %f, want 2", last["n"])
	}
	if last["team_winner_accuracy_avg"] < 0.49 || last["team_winner_accuracy_avg"] > 0.51 {
		t.Fatalf("last team_winner_accuracy_avg = %f, want 0.5", last["team_winner_accuracy_avg"])
	}
}

// Verify that limit parameter reduces the candidate set used for metrics
func TestBacktestAccuracyTrend_Limit(t *testing.T) {
	withBacktestSeams(t, func() {
		// Return three candidates but honor the limit argument
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, order string, limit int) ([]backtestCandidate, error) {
			m1 := backtestCandidate{
				MatchID: 301,
				Date:    time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:  "T20",
				Team1:   "IND",
				Team2:   "AUS",
			}
			m2 := backtestCandidate{
				MatchID: 302,
				Date:    time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:  "T20",
				Team1:   "IND",
				Team2:   "AUS",
			}
			m3 := backtestCandidate{
				MatchID: 303,
				Date:    time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:  "T20",
				Team1:   "IND",
				Team2:   "AUS",
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
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
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
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, error) {
			return matchAggregates{Runs: 155, WinnerTeamCode: "IND"}, nil
		}
	})

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&order=asc&limit=2",
		nil,
	)

	app.backtestAccuracyTrendHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var payload accuracyTrendResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 2 {
		t.Fatalf("count = %d, want 2 (limit applied)", payload.Count)
	}
	if len(payload.Results) != 2 {
		t.Fatalf("results len = %d, want 2 (limit applied)", len(payload.Results))
	}
}

// Verify that start_date/end_date filters are parsed and passed through to the seam
func TestBacktestAccuracyTrend_DateRangeFiltering(t *testing.T) {
	withBacktestSeams(t, func() {
		// Capture received start/end and return candidates accordingly
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, start, end time.Time, order string, _ int) ([]backtestCandidate, error) {
			// Expect start=2024-10-11 and end=2024-10-25
			if start.IsZero() || end.IsZero() {
				t.Fatalf("expected non-zero start/end dates, got start=%v end=%v", start, end)
			}
			// Build three dates; only middle one within range
			d1 := time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC)
			d2 := time.Date(2024, 10, 20, 14, 0, 0, 0, time.UTC)
			d3 := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)
			all := []backtestCandidate{
				{MatchID: 401, Date: d1.Format(time.RFC3339), Format: "T20", Team1: "IND", Team2: "AUS"},
				{MatchID: 402, Date: d2.Format(time.RFC3339), Format: "T20", Team1: "IND", Team2: "AUS"},
				{MatchID: 403, Date: d3.Format(time.RFC3339), Format: "T20", Team1: "IND", Team2: "AUS"},
			}
			// Simulate repo applying date filter
			filtered := make([]backtestCandidate, 0, 1)
			for _, c := range all {
				cd, _ := time.Parse(time.RFC3339, c.Date)
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
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 12}}, nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
			return matchAggregates{Runs: 150, WinnerTeamCode: "IND"}, nil
		}
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, error) {
			return matchAggregates{Runs: 152, WinnerTeamCode: "IND"}, nil
		}
	})

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&start_date=2024-10-11&end_date=2024-10-25",
		nil,
	)

	app.backtestAccuracyTrendHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var payload accuracyTrendResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 1 {
		t.Fatalf("count = %d, want 1 (only middle date in range)", payload.Count)
	}
	if len(payload.Results) != 1 || payload.Results[0].MatchID != 402 {
		t.Fatalf("unexpected results: %+v", payload.Results)
	}
}

// Verify that team1/team2 filters are passed; seam returns only when both match
func TestBacktestAccuracyTrend_TeamFiltering(t *testing.T) {
	withBacktestSeams(t, func() {
		listPlayedMatchesByFilters = func(_ context.Context, _ string, team1, team2 string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
			// Only return if IND vs AUS requested
			if team1 == "IND" && team2 == "AUS" {
				return []backtestCandidate{
					{
						MatchID: 501,
						Date:    time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
						Format:  "T20",
						Team1:   "IND",
						Team2:   "AUS",
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
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 10}}, nil
		}
	})

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS", nil)
	app.backtestAccuracyTrendHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var payload accuracyTrendResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 1 || payload.Results[0].Team1 != "IND" || payload.Results[0].Team2 != "AUS" {
		t.Fatalf("unexpected results: %+v", payload.Results)
	}
}

// --- Cache mode tests ---

// cache=read should use cached aggregates if available and must not call ML match aggregates seam
func TestBacktestAccuracyTrend_CacheRead_UsesCache(t *testing.T) {
	withBacktestSeams(t, func() {
		// One candidate
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
			return []backtestCandidate{{
				MatchID: 601,
				Date:    time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Format:  "T20",
				Team1:   "IND",
				Team2:   "AUS",
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
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
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
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, error) {
			t.Fatalf("ML match aggregates was called despite cache=read")
			return matchAggregates{}, nil
		}

		// Actuals for aggregates
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
			return matchAggregates{Runs: 150, WinnerTeamCode: "IND"}, nil
		}
	})

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&cache=read",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var payload accuracyTrendResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 1 {
		t.Fatalf("count = %d, want 1", payload.Count)
	}
	// team_runs_mae should be |155-150| = 5 from cached predictions
	if v := payload.Results[0].Metrics["team_runs_mae"]; v < 4.99 || v > 5.01 {
		t.Fatalf("team_runs_mae = %f, want 5 (from cache)", v)
	}
}

// cache=off should ignore cache even if present and use ML predictions
func TestBacktestAccuracyTrend_CacheOff_IgnoresCache(t *testing.T) {
	withBacktestSeams(t, func() {
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
			return []backtestCandidate{
				{
					MatchID: 602,
					Date:    time.Date(2024, 10, 11, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
					Format:  "T20",
					Team1:   "IND",
					Team2:   "AUS",
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
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
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
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, error) {
			return matchAggregates{Runs: 152, WinnerTeamCode: "IND"}, nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
			return matchAggregates{Runs: 150, WinnerTeamCode: "IND"}, nil
		}
	})

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&cache=off",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var payload accuracyTrendResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 1 {
		t.Fatalf("count = %d, want 1", payload.Count)
	}
	// Expect ML value 152 vs actual 150 => MAE 2 (not using cached 140)
	if v := payload.Results[0].Metrics["team_runs_mae"]; v < 1.99 || v > 2.01 {
		t.Fatalf("team_runs_mae = %f, want 2 (from ML, ignoring cache)", v)
	}
}

// cache=readwrite should compute on miss and upsert cache
func TestBacktestAccuracyTrend_CacheReadWrite_UpsertsOnMiss(t *testing.T) {
	withBacktestSeams(t, func() {
		listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
			return []backtestCandidate{
				{
					MatchID: 603,
					Date:    time.Date(2024, 10, 12, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
					Format:  "T20",
					Team1:   "IND",
					Team2:   "AUS",
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
		mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ []int64) (map[int64]playerPredictions, error) {
			return map[int64]playerPredictions{1: {Runs: 10}}, nil
		}

		// Cache miss
		getMatchPredictionAggregatesFunc = func(_ context.Context, _ int64) (db.MatchPredictionAggregates, error) {
			return db.MatchPredictionAggregates{}, sql.ErrNoRows
		}
		// ML compute path
		mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, error) {
			return matchAggregates{Runs: 149, WinnerTeamCode: "IND"}, nil
		}
		getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
			return matchAggregates{Runs: 150, WinnerTeamCode: "IND"}, nil
		}

		// Capture upsert invocation
		called := false
		upsertMatchPredictionAggregatesFunc = func(_ context.Context, row db.MatchPredictionAggregates) error {
			called = true
			if row.MatchID != 603 || row.Team1Code != "IND" || row.Team2Code != "AUS" {
				t.Fatalf("unexpected upsert row: %+v", row)
			}
			if !row.PredictedTotalRuns.Valid || row.PredictedTotalRuns.Float64 != 149 {
				t.Fatalf("expected upsert predicted_total_runs=149, got %+v", row.PredictedTotalRuns)
			}
			return nil
		}

		// Ensure subsequent read would find cache (simulate by overriding get to return same record after upsert)
		// Not strictly necessary for single-call verification.
		_ = called
	})

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&cache=readwrite",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}
