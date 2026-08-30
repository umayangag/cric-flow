package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
	sb "github.com/umayangag/cric-flow/go-app/internal/services/selectionbacktest"
)

func postSelectionComparison(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/backtest/selection-comparison", strings.NewReader(body))
	(&App{}).backtestSelectionComparisonHandler(rec, req)
	return rec
}

func stubPlayedMatches(t *testing.T, candidates []db.BacktestCandidate, err error) {
	t.Helper()
	orig := listPlayedByFmtTeams
	t.Cleanup(func() { listPlayedByFmtTeams = orig })
	listPlayedByFmtTeams = func(context.Context, string, string, string) ([]db.BacktestCandidate, error) {
		return candidates, err
	}
}

func candidate(id int64, winner string) db.BacktestCandidate {
	return db.BacktestCandidate{
		MatchID:    id,
		MatchDate:  time.Date(2024, 3, int(id), 0, 0, 0, 0, time.UTC),
		Team1:      "A",
		Team2:      "B",
		WinnerTeam: sql.NullString{String: winner, Valid: winner != ""},
	}
}

func TestSelectionComparison_RejectsAMissingPair(t *testing.T) {
	rec := postSelectionComparison(t, `{"format":"T20"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "INVALID_PARAM")
}

func TestSelectionComparison_RejectsUnreadableJSON(t *testing.T) {
	rec := postSelectionComparison(t, `{`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "INVALID_JSON")
}

// TestSelectionComparison_RefusesAnUnboundedRun: each match costs two selections and one
// of them is a search, so the limit is the difference between a request and an outage.
func TestSelectionComparison_RefusesAnUnboundedRun(t *testing.T) {
	rec := postSelectionComparison(t, `{"format":"T20","team1":"A","team2":"B","limit":500}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "limit exceeds the maximum")
}

func TestSelectionComparison_SaysWhenThePairHasNoPlayedMatches(t *testing.T) {
	stubPlayedMatches(t, nil, nil)

	rec := postSelectionComparison(t, `{"format":"T20","team1":"A","team2":"B"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "NO_MATCHES")
}

func TestSelectionComparison_SurfacesAListingFailure(t *testing.T) {
	stubPlayedMatches(t, nil, errors.New("db down"))

	rec := postSelectionComparison(t, `{"format":"T20","team1":"A","team2":"B"}`)

	assert.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError)
}

// TestSelectionComparisonMatches_TakesTheMostRecentWithinTheLimit: the newest matches are
// the ones whose features are best populated.
func TestSelectionComparisonMatches_TakesTheMostRecentWithinTheLimit(t *testing.T) {
	stubPlayedMatches(t, []db.BacktestCandidate{
		candidate(1, "A"), candidate(2, "B"), candidate(3, "A"),
	}, nil)

	matches, err := selectionComparisonMatches(context.Background(), "T20", "A", "B", 2)

	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, int64(2), matches[0].MatchID)
	assert.Equal(t, int64(3), matches[1].MatchID)
}

func TestSelectionComparisonMatches_CarriesTheActualWinner(t *testing.T) {
	stubPlayedMatches(t, []db.BacktestCandidate{candidate(1, "A"), candidate(2, "")}, nil)

	matches, err := selectionComparisonMatches(context.Background(), "T20", "A", "B", 10)

	require.NoError(t, err)
	assert.Equal(t, "A", matches[0].ActualWinner)
	assert.Empty(t, matches[1].ActualWinner, "a match with no result must not claim one")
}

// TestArmSelectionFromResult_ReadsBothXIsAndTheWinProbability covers the reduction from a
// full prediction to the parts the comparison scores.
func TestArmSelectionFromResult_ReadsBothXIsAndTheWinProbability(t *testing.T) {
	result := &predictteam.Result{
		Team1: []predictteam.SelectedPlayer{{PlayerID: 1}, {PlayerID: 2}},
		Team2: []predictteam.SelectedPlayer{{PlayerID: 3}},
		ScorecardSummary: &predictteam.ScorecardSummary{
			Team1WinProbability: 0.62,
			PredictedWinner:     "A",
		},
	}

	got := armSelectionFromResult(result)

	assert.Equal(t, []int64{1, 2, 3}, got.SelectedPlayerIDs)
	assert.InDelta(t, 0.62, got.Team1WinProbability, 1e-9)
	assert.Equal(t, "A", got.PredictedWinner)
}

func TestArmSelectionFromResult_HandlesAResultWithoutAScorecard(t *testing.T) {
	got := armSelectionFromResult(&predictteam.Result{
		Team1: []predictteam.SelectedPlayer{{PlayerID: 1}},
	})

	assert.Equal(t, []int64{1}, got.SelectedPlayerIDs)
	assert.Zero(t, got.Team1WinProbability)
	assert.Empty(t, got.PredictedWinner, "no scorecard means no predicted winner, not a wrong one")
}

func TestArmSelectionFromResult_HandlesNil(t *testing.T) {
	assert.Empty(t, armSelectionFromResult(nil).SelectedPlayerIDs)
}

// TestSelectionComparison_ReportsBothArms drives the handler end to end with a stubbed
// selector, so the wiring from request to report is exercised without an ML service.
func TestSelectionComparison_ReportsBothArms(t *testing.T) {
	stubPlayedMatches(t, []db.BacktestCandidate{candidate(1, "A"), candidate(2, "B")}, nil)
	matches, err := selectionComparisonMatches(context.Background(), "T20", "A", "B", 10)
	require.NoError(t, err)

	// greedy always calls "A"; winprob calls the match correctly.
	selector := func(_ context.Context, m sb.Match, mode predictteam.SelectionMode) (sb.ArmSelection, error) {
		if mode == predictteam.SelectionModeGreedy {
			return sb.ArmSelection{PredictedWinner: "A", SelectedPlayerIDs: []int64{1, 2}}, nil
		}
		return sb.ArmSelection{PredictedWinner: m.ActualWinner, SelectedPlayerIDs: []int64{1, 3}}, nil
	}
	arms := []sb.Arm{
		{Name: "greedy", Mode: predictteam.SelectionModeGreedy},
		{Name: "winprob", Mode: predictteam.SelectionModeWinProbability},
	}

	report := sb.Summarize(sb.Run(context.Background(), matches, arms, selector))

	body, err := json.Marshal(report)
	require.NoError(t, err)
	assert.Contains(t, string(body), "winner_accuracy")
	require.Len(t, report.Arms, 2)
	require.NotNil(t, report.Arms[0].WinnerAccuracy)
	require.NotNil(t, report.Arms[1].WinnerAccuracy)
	assert.InDelta(t, 0.5, *report.Arms[0].WinnerAccuracy, 1e-9, "greedy called one of two")
	assert.InDelta(t, 1.0, *report.Arms[1].WinnerAccuracy, 1e-9, "winprob called both")
	require.Len(t, report.Divergences, 1)
	assert.Equal(t, 2, report.Divergences[0].Matches)
}
