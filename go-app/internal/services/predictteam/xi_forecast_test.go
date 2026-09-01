package predictteam

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSimulator struct {
	result *XISimulationResult
	err    error
	req    XISimulationRequest
}

func (f *fakeSimulator) SimulateMatchXI(
	_ context.Context,
	req XISimulationRequest,
) (*XISimulationResult, error) {
	f.req = req
	return f.result, f.err
}

type fakePerformance struct {
	result *XIPerformanceResult
	err    error
}

func (f *fakePerformance) PredictPerformance(
	_ context.Context,
	_ XIPerformanceRequest,
) (*XIPerformanceResult, error) {
	return f.result, f.err
}

func simulatedPlayer(id int64) XISimulatedPlayer {
	return XISimulatedPlayer{
		PlayerID:              id,
		Runs:                  XISimulatedRange{P10: 4, Median: 21, P90: 55},
		BallsFaced:            XISimulatedRange{P10: 5, Median: 17, P90: 38},
		Wickets:               XISimulatedRange{P10: 0, Median: 1, P90: 3},
		RunsConceded:          XISimulatedRange{P10: 12, Median: 28, P90: 47},
		ScorecardRuns:         23,
		ScorecardBalls:        18,
		ScorecardWickets:      1.2,
		ScorecardRunsConceded: 30,
		ScorecardBallsBowled:  24,
		SpreadShare:           0.12,
	}
}

func simulatedSide(id int64, total float64) XISimulatedSide {
	return XISimulatedSide{
		Total:           XISimulatedRange{P10: total - 40, Median: total, P90: total + 40},
		TotalScorecard:  total + 1,
		ExtrasScorecard: 9,
		Players:         []XISimulatedPlayer{simulatedPlayer(id)},
	}
}

func simulationResult(headlineSource string) *XISimulationResult {
	return &XISimulationResult{
		Samples:                      2000,
		TossMarginalised:             true,
		Team1:                        simulatedSide(1, 170),
		Team2:                        simulatedSide(4, 160),
		SimulatedTeam1WinProbability: 0.58,
		DisplayTeam1WinProbability:   0.61,
		HeadlineTeam1WinProbability:  0.61,
		HeadlineSource:               headlineSource,
	}
}

func resultWithOnePlayerEachSide() *Result {
	return &Result{
		Team1: []SelectedPlayer{{PlayerID: 1, PlayerName: "A"}},
		Team2: []SelectedPlayer{{PlayerID: 4, PlayerName: "B"}},
	}
}

func TestApplyXISimulation_WritesRangesTotalsAndSpreadFromOneSetOfDraws(t *testing.T) {
	t.Parallel()
	simulator := &fakeSimulator{result: simulationResult(winProbabilitySourceDisplay)}
	result := resultWithOnePlayerEachSide()
	fix := twoSidedFixture("T20")

	err := applyXISimulation(context.Background(), simulator, fix, []int64{1}, []int64{4}, result)

	require.NoError(t, err)
	player := result.Team1[0]
	assert.Equal(t, 23.0, player.Runs, "the point shown is the median-band scorecard line")
	assert.Equal(t, &ValueRange{P10: 4, P90: 55}, player.RunsRange)
	assert.Equal(t, &ValueRange{P10: 0, P90: 3}, player.WicketsRange)
	assert.Equal(t, &ValueRange{P10: 12, P90: 47}, player.RunsConcededRange)
	assert.InDelta(t, 7.5, player.Economy, 1e-9, "30 runs off 24 balls is 7.5 an over")
	require.NotNil(t, player.SpreadShare)
	assert.InDelta(t, 0.12, *player.SpreadShare, 1e-9)

	require.NotNil(t, result.Scorecard)
	assert.Equal(t, 2000, result.Scorecard.Samples)
	assert.Equal(t, InningsTotal{Total: 171, Extras: 9, P10: 130, Median: 170, P90: 210}, result.Scorecard.Innings1)
	assert.Equal(t, 0.61, result.WinProbability.Team1)
	assert.Equal(t, winProbabilitySourceDisplay, result.WinProbability.Source)
	require.NotNil(t, result.WinProbability.Simulated)
	assert.Equal(t, 0.58, *result.WinProbability.Simulated, "the other model is reported, never blended")
	assert.Equal(t, "IND", result.WinProbability.PredictedWinner)
}

func TestApplyXISimulation_RefusesAWinProbabilityWithNoHonestSource(t *testing.T) {
	t.Parallel()
	simulator := &fakeSimulator{result: simulationResult("vibes")}

	err := applyXISimulation(context.Background(), simulator, twoSidedFixture("T20"),
		[]int64{1}, []int64{4}, resultWithOnePlayerEachSide())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown win-probability source")
}

func TestApplyXISimulation_FailsTheRequestRatherThanAnsweringWithoutNumbers(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		simulator *fakeSimulator
		want      string
	}{
		{
			name:      "the call failed",
			simulator: &fakeSimulator{err: errors.New("performance model not loaded")},
			want:      "performance model not loaded",
		},
		{
			name:      "the simulator returned no players",
			simulator: &fakeSimulator{result: &XISimulationResult{HeadlineSource: winProbabilitySourceSimulator}},
			want:      "returned no players",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := applyXISimulation(context.Background(), tc.simulator, twoSidedFixture("T20"),
				[]int64{1}, []int64{4}, resultWithOnePlayerEachSide())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestApplyPerformanceForecast_WritesMediansAndRangesAndNoTotals(t *testing.T) {
	t.Parallel()
	predictor := &fakePerformance{result: &XIPerformanceResult{
		InningsMarginalised: true,
		Players: []XIPerformancePlayer{
			{
				PlayerID:     1,
				Runs:         XISimulatedRange{P10: 3, Median: 26, P90: 71},
				BallsFaced:   XISimulatedRange{P10: 8, Median: 44, P90: 110},
				RunsConceded: XISimulatedRange{P10: 10, Median: 33, P90: 60},
				Wickets:      1.4,
			},
		},
	}}
	result := resultWithOnePlayerEachSide()

	err := applyPerformanceForecast(context.Background(), predictor, twoSidedFixture("TEST"),
		[]int64{1}, []int64{4}, result)

	require.NoError(t, err)
	assert.Equal(t, 26.0, result.Team1[0].Runs)
	assert.Equal(t, &ValueRange{P10: 3, P90: 71}, result.Team1[0].RunsRange)
	assert.Equal(t, 1.4, result.Team1[0].Wickets)
	assert.Equal(t, 33.0, result.Team1[0].RunsConceded)
	assert.Zero(t, result.Team1[0].Economy, "economy needs balls bowled, which L2-B does not forecast")
	assert.Nil(t, result.Scorecard, "no innings length means no innings total, and none is invented")
	assert.Zero(t, result.Team2[0].Runs, "a player the forecast omitted keeps his zeros")
}

func TestApplyPerformanceForecast_PropagatesTheFailure(t *testing.T) {
	t.Parallel()
	predictor := &fakePerformance{err: errors.New("no performance model for TEST")}

	err := applyPerformanceForecast(context.Background(), predictor, twoSidedFixture("TEST"),
		[]int64{1}, []int64{4}, resultWithOnePlayerEachSide())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no performance model for TEST")
}

func TestFormatHasInningsLength_OnlyLimitedOversFormatsAreSimulated(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		format string
		want   bool
	}{
		{name: "T20", format: "T20", want: true},
		{name: "T20I", format: "t20i", want: true},
		{name: "ODI", format: " ODI ", want: true},
		{name: "TEST has no innings length", format: "TEST", want: false},
		{name: "an unknown format", format: "HUNDRED", want: false},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, formatHasInningsLength(tc.format))
		})
	}
}

func TestEconomy_IsZeroForAPlayerWhoDidNotBowl(t *testing.T) {
	t.Parallel()
	assert.Zero(t, economy(30, 0))
	assert.InDelta(t, 6.0, economy(30, 30), 1e-9)
}

func TestWinnerFrom_NamesTeam2OnAnExactTie(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "IND", winnerFrom(0.5, "IND", "AUS"))
	assert.Equal(t, "AUS", winnerFrom(0.4999, "IND", "AUS"))
}

func TestNewSelectedPlayers_NamesPlayersAndAttachesMarginalValues(t *testing.T) {
	t.Parallel()
	players := newSelectedPlayers([]int64{2, 1}, pool(1, 2, 3), map[int64]float64{2: 0.03})

	require.Len(t, players, 2)
	assert.Equal(t, int64(2), players[0].PlayerID)
	require.NotNil(t, players[0].MarginalValue)
	assert.InDelta(t, 0.03, *players[0].MarginalValue, 1e-9)
	assert.Nil(t, players[1].MarginalValue, "a player with no marginal value carries none")
}
