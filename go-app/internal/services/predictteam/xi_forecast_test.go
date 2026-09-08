package predictteam

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/teams"
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

func simulatedPlayer(key string) XISimulatedPlayer {
	return XISimulatedPlayer{
		PlayerKey:             key,
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

func simulatedSide(key string, total float64) XISimulatedSide {
	return XISimulatedSide{
		Total:           XISimulatedRange{P10: total - 40, Median: total, P90: total + 40},
		TotalScorecard:  total + 1,
		ExtrasScorecard: 9,
		Players:         []XISimulatedPlayer{simulatedPlayer(key)},
	}
}

func simulationResult(headlineSource string) *XISimulationResult {
	return &XISimulationResult{
		Samples:                      2000,
		TossMarginalised:             true,
		Team1:                        simulatedSide("a1", 170),
		Team2:                        simulatedSide("b1", 160),
		SimulatedTeam1WinProbability: 0.58,
		DisplayTeam1WinProbability:   0.61,
		HeadlineTeam1WinProbability:  0.61,
		HeadlineSource:               headlineSource,
		Served:                       servedFromRunA,
	}
}

func resultWithOnePlayerEachSide() *Result {
	return &Result{
		Team1: []SelectedPlayer{{PlayerID: 1, PlayerKey: "a1", PlayerName: "A"}},
		Team2: []SelectedPlayer{{PlayerID: 4, PlayerKey: "b1", PlayerName: "B"}},
	}
}

func TestApplyXISimulation_WritesRangesTotalsAndSpreadFromOneSetOfDraws(t *testing.T) {
	t.Parallel()
	simulator := &fakeSimulator{result: simulationResult(winProbabilitySourceDisplay)}
	result := resultWithOnePlayerEachSide()
	fix := twoSidedFixture("T20")

	err := applyXISimulation(context.Background(), simulator, fix, []string{"a1"}, []string{"b1"}, result)

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
	assert.Equal(t, servedFromRunA, result.ServedRatings, "the draws stamp the prediction they fill")
	assert.Equal(t, 2000, result.Scorecard.Samples)
	assert.Equal(t, InningsTotal{Total: 171, Extras: 9, P10: 130, Median: 170, P90: 210}, result.Scorecard.Team1Innings)
	assert.Equal(t, 0.61, result.WinProbability.Team1)
	assert.Equal(t, winProbabilitySourceDisplay, result.WinProbability.Source)
	require.NotNil(t, result.WinProbability.Simulated)
	assert.Equal(t, 0.58, *result.WinProbability.Simulated, "the other model is reported, never blended")
	assert.Equal(t, "India (men)", result.WinProbability.PredictedWinner)
}

func TestApplyXISimulation_RefusesAWinProbabilityWithNoHonestSource(t *testing.T) {
	t.Parallel()
	simulator := &fakeSimulator{result: simulationResult("vibes")}

	err := applyXISimulation(context.Background(), simulator, twoSidedFixture("T20"),
		[]string{"a1"}, []string{"b1"}, resultWithOnePlayerEachSide())

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
				[]string{"a1"}, []string{"b1"}, resultWithOnePlayerEachSide())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestApplyPerformanceForecast_WritesMediansAndRangesAndNoTotals(t *testing.T) {
	t.Parallel()
	predictor := &fakePerformance{result: &XIPerformanceResult{
		InningsMarginalised: true,
		Served:              servedFromRunA,
		Players: []XIPerformancePlayer{
			{
				PlayerKey:    "a1",
				Runs:         XISimulatedRange{P10: 3, Median: 26, P90: 71},
				BallsFaced:   XISimulatedRange{P10: 8, Median: 44, P90: 110},
				RunsConceded: XISimulatedRange{P10: 10, Median: 33, P90: 60},
				Wickets:      1.4,
			},
			{PlayerKey: "b1", Runs: XISimulatedRange{P10: 1, Median: 12, P90: 40}},
		},
	}}
	result := resultWithOnePlayerEachSide()

	err := applyPerformanceForecast(context.Background(), predictor, twoSidedFixture("TEST"),
		[]string{"a1"}, []string{"b1"}, result)

	require.NoError(t, err)
	assert.Equal(t, 26.0, result.Team1[0].Runs)
	assert.Equal(t, &ValueRange{P10: 3, P90: 71}, result.Team1[0].RunsRange)
	assert.Equal(t, 1.4, result.Team1[0].Wickets)
	assert.Equal(t, 33.0, result.Team1[0].RunsConceded)
	assert.Zero(t, result.Team1[0].Economy, "economy needs balls bowled, which L2-B does not forecast")
	assert.Nil(t, result.Scorecard, "no innings length means no innings total, and none is invented")
	// §8.7: the substitution is on the wire, not only in a log line.
	assert.Equal(t, "performance_quantiles", result.Forecast.Source)
	assert.Contains(t, result.Forecast.Note, "no innings length")
	assert.Equal(t, servedFromRunA, result.ServedRatings, "the forecast stamps the prediction it fills")
}

// The forecast has to come from the rating state the XIs were chosen from: a result already
// stamped by the selection refuses draws or quantiles from another run (P1-5).
func TestApplyMatchForecast_RefusesAForecastFromAnotherRunThanTheSelection(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		apply func(result *Result) error
	}{
		{
			name: "the simulator answered from a new run",
			apply: func(result *Result) error {
				sim := simulationResult(winProbabilitySourceDisplay)
				sim.Served = servedFromRunB
				return applyXISimulation(context.Background(), &fakeSimulator{result: sim},
					twoSidedFixture("T20"), []string{"a1"}, []string{"b1"}, result)
			},
		},
		{
			name: "the performance model answered from a new run",
			apply: func(result *Result) error {
				forecast := &XIPerformanceResult{
					Served:  servedFromRunB,
					Players: []XIPerformancePlayer{{PlayerKey: "a1"}, {PlayerKey: "b1"}},
				}
				return applyPerformanceForecast(context.Background(), &fakePerformance{result: forecast},
					twoSidedFixture("TEST"), []string{"a1"}, []string{"b1"}, result)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := resultWithOnePlayerEachSide()
			result.ServedRatings = servedFromRunA

			err := tc.apply(result)

			var changed *ServedRunChangedError
			require.ErrorAs(t, err, &changed)
			assert.Equal(t, servedFromRunA, changed.Was)
			assert.Equal(t, servedFromRunB, changed.Now)
			assert.Equal(t, servedFromRunA, result.ServedRatings, "the stamp is not overwritten by the refused answer")
		})
	}
}

// TestApplyPerformanceForecast_RefusesAPlayerItHasNoForecastFor: a row left at zeros reads
// as a forecast of nothing rather than as a missing forecast, and no field says which.
func TestApplyPerformanceForecast_RefusesAPlayerItHasNoForecastFor(t *testing.T) {
	t.Parallel()
	predictor := &fakePerformance{result: &XIPerformanceResult{
		Served:  servedFromRunA,
		Players: []XIPerformancePlayer{{PlayerKey: "a1", Runs: XISimulatedRange{Median: 26}}},
	}}

	err := applyPerformanceForecast(context.Background(), predictor, twoSidedFixture("TEST"),
		[]string{"a1"}, []string{"b1"}, resultWithOnePlayerEachSide())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `no forecast for selected player "b1"`)
}

func TestApplyPerformanceForecast_PropagatesTheFailure(t *testing.T) {
	t.Parallel()
	predictor := &fakePerformance{err: errors.New("no performance model for TEST")}

	err := applyPerformanceForecast(context.Background(), predictor, twoSidedFixture("TEST"),
		[]string{"a1"}, []string{"b1"}, resultWithOnePlayerEachSide())

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
			assert.Equal(t, tc.want, FormatHasInningsLength(tc.format))
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
	assert.Equal(t, "India (men)", winnerFrom(0.5, indiaMen, australiaMen))
	assert.Equal(t, "Australia (men)", winnerFrom(0.4999, indiaMen, australiaMen))
}

// The winner is named as the side that was scored, not as the caller spelled it: two teams
// answer to "India", and a result that says only "India" does not say which one won (D-10).
func TestWinnerFrom_NamesTheResolvedSideNotTheTypedName(t *testing.T) {
	t.Parallel()
	indiaWomen := db.TeamSide{ClubID: 132, Name: "India", Gender: teams.GenderFemale}

	assert.Equal(t, "India (women)", winnerFrom(0.7, indiaWomen, australiaMen))
}

func TestNewSelectedPlayers_NamesPlayersAndAttachesMarginalValues(t *testing.T) {
	t.Parallel()
	players := newSelectedPlayers([]string{"k2", "k1"}, pool(1, 2, 3), map[string]float64{"k2": 0.03}, nil)

	require.Len(t, players, 2)
	assert.Equal(t, int64(2), players[0].PlayerID)
	require.NotNil(t, players[0].MarginalValue)
	assert.InDelta(t, 0.03, *players[0].MarginalValue, 1e-9)
	assert.Nil(t, players[1].MarginalValue, "a player with no marginal value carries none")
}

func TestNewSelectedPlayers_DropsAKeyNoPoolRowClaims(t *testing.T) {
	t.Parallel()
	players := newSelectedPlayers([]string{"k1", "ghost"}, pool(1, 2), nil, nil)

	require.Len(t, players, 1, "a key with no pool row names nobody and is not invented")
	assert.Equal(t, int64(1), players[0].PlayerID)
}

// The toss is an input now (P1-1), and these three states are what the control offers.
// Each has to reach the simulator, and the response has to say which one it assumed.
func TestApplyXISimulation_CarriesEachTossStateToTheSimulatorAndNamesIt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		team1BatsFirst   *bool
		tossMarginalised bool
	}{
		{name: "unknown draws both batting orders", team1BatsFirst: nil, tossMarginalised: true},
		{name: "team1 bats first", team1BatsFirst: boolValue(true), tossMarginalised: false},
		{name: "team2 bats first", team1BatsFirst: boolValue(false), tossMarginalised: false},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sim := simulationResult(winProbabilitySourceDisplay)
			sim.TossMarginalised = tc.tossMarginalised
			simulator := &fakeSimulator{result: sim}
			fix := twoSidedFixture("T20")
			fix.team1BatsFirst = tc.team1BatsFirst
			result := resultWithOnePlayerEachSide()

			err := applyXISimulation(context.Background(), simulator, fix, []string{"a1"}, []string{"b1"}, result)

			require.NoError(t, err)
			assert.Equal(t, tc.team1BatsFirst, simulator.req.Team1BatsFirst, "the toss reaches /simulate")
			assert.Equal(t, tc.team1BatsFirst, result.Toss.Team1BatsFirst, "and the response names it")
			assert.True(t, result.Toss.Honoured)
			assert.Empty(t, result.Toss.Note)
			require.NotNil(t, result.Scorecard)
			assert.Equal(t, tc.tossMarginalised, result.Scorecard.TossMarginalised)
		})
	}
}

// A known toss answered by marginalised draws is the request being silently changed, and
// `toss_marginalised` is the one field that can catch it (§8.7).
func TestApplyXISimulation_RefusesDrawsThatIgnoredTheTossAsked(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		team1BatsFirst   *bool
		tossMarginalised bool
		want             string
	}{
		{
			name:             "a named toss answered by both orders",
			team1BatsFirst:   boolValue(true),
			tossMarginalised: true,
			want:             "the toss was team1 bats first",
		},
		{
			name:             "an unknown toss answered by one order",
			team1BatsFirst:   nil,
			tossMarginalised: false,
			want:             "the toss was unknown",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sim := simulationResult(winProbabilitySourceDisplay)
			sim.TossMarginalised = tc.tossMarginalised
			fix := twoSidedFixture("T20")
			fix.team1BatsFirst = tc.team1BatsFirst

			err := applyXISimulation(context.Background(), &fakeSimulator{result: sim}, fix,
				[]string{"a1"}, []string{"b1"}, resultWithOnePlayerEachSide())

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// A format with no innings length has no batting order to fix, and the response says the
// toss was not used rather than returning numbers that quietly ignored it (§8.7).
func TestTossNotSimulated_SaysANamedTossWasNotUsed(t *testing.T) {
	t.Parallel()

	unknown := tossNotSimulated(nil)
	assert.Nil(t, unknown.Team1BatsFirst)
	assert.True(t, unknown.Honoured, "asking for nothing and getting nothing is honoured")
	assert.Empty(t, unknown.Note)

	named := tossNotSimulated(boolValue(true))
	assert.Nil(t, named.Team1BatsFirst, "there was no batting order to fix, so none is claimed")
	assert.False(t, named.Honoured)
	assert.Contains(t, named.Note, "no innings length")
}

func TestTossDescription_NamesEachState(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unknown", tossDescription(nil))
	assert.Equal(t, "team1 bats first", tossDescription(boolValue(true)))
	assert.Equal(t, "team2 bats first", tossDescription(boolValue(false)))
}

func boolValue(v bool) *bool {
	return &v
}
