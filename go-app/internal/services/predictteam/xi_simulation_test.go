package predictteam

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSimulator implements MLPredictor (minimally), EnhancedWinPredictor and XISimulator,
// answering /simulate with a fixed result and recording the request.
type fakeSimulator struct {
	result   *XISimulationResult
	err      error
	requests []XISimulationRequest
}

func (f *fakeSimulator) PredictPlayers(context.Context, time.Time, string, []int64, map[int64]map[string]float64, *MatchContext) (map[int64]PlayerPred, error) {
	return nil, nil
}

func (f *fakeSimulator) PredictMatchWin(context.Context, WinFeatures) (float64, error) {
	return 0.5, nil
}

func (f *fakeSimulator) SimulateMatchXI(_ context.Context, req XISimulationRequest) (*XISimulationResult, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// nonSimulatingPredictor implements MLPredictor only.
type nonSimulatingPredictor struct{}

func (nonSimulatingPredictor) PredictPlayers(context.Context, time.Time, string, []int64, map[int64]map[string]float64, *MatchContext) (map[int64]PlayerPred, error) {
	return nil, nil
}

func (nonSimulatingPredictor) PredictMatchWin(context.Context, WinFeatures) (float64, error) {
	return 0.5, nil
}

func simulatedSideFixture(ids []int64, total float64) XISimulatedSide {
	players := make([]XISimulatedPlayer, 0, len(ids))
	share := 1.0 / float64(len(ids)+1)
	each := (total - 8) / float64(len(ids))
	for _, id := range ids {
		players = append(players, XISimulatedPlayer{
			PlayerID:              id,
			Runs:                  XISimulatedRange{P10: each - 10, Median: each, P90: each + 20},
			Wickets:               XISimulatedRange{P10: 0, Median: 1, P90: 2},
			ScorecardRuns:         each,
			ScorecardBalls:        each * 0.8,
			ScorecardWickets:      0.9,
			ScorecardRunsConceded: 24,
			ScorecardBallsBowled:  18,
			SpreadShare:           share,
		})
	}
	return XISimulatedSide{
		Total:           XISimulatedRange{P10: total - 30, Median: total, P90: total + 30},
		TotalMean:       total,
		TotalScorecard:  total,
		ExtrasScorecard: 8,
		Players:         players,
	}
}

func selectedFixture(ids []int64) []SelectedPlayer {
	out := make([]SelectedPlayer, 0, len(ids))
	for _, id := range ids {
		out = append(out, SelectedPlayer{PlayerID: id, Runs: 1, Wickets: 1, Economy: 9})
	}
	return out
}

func TestApplyXISimulation_ScorecardComesFromTheSimulatorAndSumsToItsTotal(t *testing.T) {
	// Arrange
	withXIWinModel(t, true)
	ids1, ids2 := []int64{1, 2, 3}, []int64{4, 5, 6}
	sim := &fakeSimulator{result: &XISimulationResult{
		Samples:                      2000,
		TossMarginalised:             true,
		Team1:                        simulatedSideFixture(ids1, 158),
		Team2:                        simulatedSideFixture(ids2, 149),
		SimulatedTeam1WinProbability: 0.58,
		DisplayTeam1WinProbability:   0.61,
		HeadlineTeam1WinProbability:  0.61,
		HeadlineSource:               "display",
	}}
	result := &Result{Team1: selectedFixture(ids1), Team2: selectedFixture(ids2)}
	summary := ScorecardSummary{Innings1Total: 999, Innings2Total: 999, Team1WinProbability: 0.61}
	inputs := xiScorecardInputs{
		format: "t20", team1IDs: ids1, team2IDs: ids2, team1ID: 7, team2ID: 8, venueID: 3,
		team1Code: "IND", team2Code: "AUS", marginalValues: map[int64]float64{1: 0.02, 4: 0.01},
	}

	// Act
	applied := applyXISimulation(context.Background(), sim, inputs, result, &summary)

	// Assert
	require.True(t, applied)
	require.Len(t, sim.requests, 1)
	assert.Equal(t, "T20", sim.requests[0].Format)
	assert.Equal(t, ids1, sim.requests[0].Team1PlayerIDs)
	assert.Equal(t, int64(3), sim.requests[0].VenueID)
	var runs1 float64
	for _, p := range result.Team1 {
		runs1 += p.Runs
	}
	assert.InDelta(t, summary.Innings1Total, runs1+summary.ExtrasInnings1, 1e-9, "lines plus extras equal the total")
	assert.InDelta(t, 158, summary.Innings1Total, 1e-9)
	assert.InDelta(t, 149, summary.Innings2Total, 1e-9)
	assert.InDelta(t, 0.61, summary.Team1WinProbability, 1e-9, "the display model stays the headline")
	assert.Equal(t, "IND", summary.PredictedWinner)
	require.NotNil(t, result.Team1[0].RunsRange)
	assert.InDelta(t, 40, result.Team1[0].RunsRange.P10, 1e-9)
	assert.InDelta(t, 8, result.Team1[0].Economy, 1e-9, "24 runs off 18 balls")
	require.NotNil(t, result.XISimulation)
	assert.Equal(t, "display", result.XISimulation.WinProbabilitySource)
	assert.InDelta(t, 0.58, result.XISimulation.SimulatedTeam1WinProbability, 1e-9)
	assert.InDelta(t, 128, result.XISimulation.Innings1.P10, 1e-9)
	require.NotNil(t, result.Explanation)
	assert.Len(t, result.Explanation.SpreadContributions, 6)
	assert.InDelta(t, 0.02, result.Explanation.MarginalValues[1], 1e-9)
}

func TestApplyXISimulation_LeavesTheScorecardAloneWhenItCannotRun(t *testing.T) {
	testCases := []struct {
		name      string
		xiEnabled bool
		predictor MLPredictor
		format    string
	}{
		{name: "xi win model off", xiEnabled: false, predictor: &fakeSimulator{result: &XISimulationResult{}}, format: "T20"},
		{name: "predictor cannot simulate", xiEnabled: true, predictor: nonSimulatingPredictor{}, format: "T20"},
		{name: "format without an innings length", xiEnabled: true, predictor: &fakeSimulator{result: &XISimulationResult{}}, format: "TEST"},
		{name: "the call fails", xiEnabled: true, predictor: &fakeSimulator{err: errors.New("boom")}, format: "ODI"},
		{name: "no players returned", xiEnabled: true, predictor: &fakeSimulator{result: &XISimulationResult{}}, format: "ODI"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			withXIWinModel(t, tc.xiEnabled)
			result := &Result{Team1: selectedFixture([]int64{1}), Team2: selectedFixture([]int64{2})}
			summary := ScorecardSummary{Innings1Total: 150, Innings2Total: 140, Team1WinProbability: 0.55}

			// Act
			applied := applyXISimulation(context.Background(), tc.predictor, xiScorecardInputs{format: tc.format}, result, &summary)

			// Assert
			assert.False(t, applied)
			assert.InDelta(t, 150, summary.Innings1Total, 1e-9)
			assert.InDelta(t, 1, result.Team1[0].Runs, 1e-9)
			assert.Nil(t, result.XISimulation)
			assert.Nil(t, result.Team1[0].RunsRange)
		})
	}
}

func TestFormatHasInningsLength_MirrorsTheSimulatedFormats(t *testing.T) {
	t.Parallel()

	assert.True(t, formatHasInningsLength(" t20 "))
	assert.True(t, formatHasInningsLength("ODI"))
	assert.True(t, formatHasInningsLength("T20I"))
	assert.False(t, formatHasInningsLength("TEST"))
	assert.False(t, formatHasInningsLength(""))
}

func TestEconomy_IsRunsPerOverAndZeroWithoutBalls(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 7.5, economy(30, 24), 1e-9)
	assert.InDelta(t, 0, economy(30, 0), 1e-9)
}

func TestXIMarginalValues_MergesBothSides(t *testing.T) {
	t.Parallel()
	m := &xiMarginalValues{}

	m.record(true, map[int64]float64{1: 0.1})
	m.record(false, map[int64]float64{2: 0.2})
	m.record(true, map[int64]float64{1: 0.3}) // a later round replaces the side's values

	assert.Equal(t, map[int64]float64{1: 0.3, 2: 0.2}, m.values())
	assert.Empty(t, (&xiMarginalValues{}).values())
}
