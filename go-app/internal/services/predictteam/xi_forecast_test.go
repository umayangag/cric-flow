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
	req    XIPerformanceRequest
}

func (f *fakePerformance) PredictPerformance(
	_ context.Context,
	req XIPerformanceRequest,
) (*XIPerformanceResult, error) {
	f.req = req
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
				Side:         Team1Side,
				Runs:         XISimulatedRange{P10: 3, Median: 26, P90: 71},
				BallsFaced:   XISimulatedRange{P10: 8, Median: 44, P90: 110},
				RunsConceded: XISimulatedRange{P10: 10, Median: 33, P90: 60},
				Wickets:      1.4,
			},
			{PlayerKey: "b1", Side: Team2Side, Runs: XISimulatedRange{P10: 1, Median: 12, P90: 40}},
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
					Served: servedFromRunB,
					// The fixture names no toss, so the forecast that answers it is the
					// marginalised one; anything else is refused before the run is read.
					InningsMarginalised: true,
					Players: []XIPerformancePlayer{
						{PlayerKey: "a1", Side: Team1Side},
						{PlayerKey: "b1", Side: Team2Side},
					},
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
		Served:              servedFromRunA,
		InningsMarginalised: true,
		Players: []XIPerformancePlayer{
			{PlayerKey: "a1", Side: Team1Side, Runs: XISimulatedRange{Median: 26}},
		},
	}}

	err := applyPerformanceForecast(context.Background(), predictor, twoSidedFixture("TEST"),
		[]string{"a1"}, []string{"b1"}, resultWithOnePlayerEachSide())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `no forecast for selected player "b1"`)
}

// TestApplyPerformanceForecast_ReadsEachSidesOwnRow: `/performance/predict` answers both
// elevens in one flat list, so a row is identified by side *and* id. Keyed by id alone, a
// player who appeared on both sides left one side reading the other's forecast (GO-04).
func TestApplyPerformanceForecast_ReadsEachSidesOwnRow(t *testing.T) {
	t.Parallel()
	predictor := &fakePerformance{result: &XIPerformanceResult{
		Served:              servedFromRunA,
		InningsMarginalised: true,
		Players: []XIPerformancePlayer{
			{PlayerKey: "shared", Side: Team1Side, Runs: XISimulatedRange{Median: 61}},
			{PlayerKey: "shared", Side: Team2Side, Runs: XISimulatedRange{Median: 12}},
		},
	}}
	result := &Result{
		Team1: []SelectedPlayer{{PlayerID: 1, PlayerKey: "shared", PlayerName: "A"}},
		Team2: []SelectedPlayer{{PlayerID: 2, PlayerKey: "shared", PlayerName: "B"}},
	}

	err := applyPerformanceForecast(context.Background(), predictor, twoSidedFixture("TEST"),
		[]string{"shared"}, []string{"shared"}, result)

	require.NoError(t, err)
	assert.Equal(t, 61.0, result.Team1[0].Runs, "team1 reads team1's row")
	assert.Equal(t, 12.0, result.Team2[0].Runs, "team2 reads team2's own, not whichever came last")
}

// TestApplyPerformanceForecast_RefusesARowFromTheWrongSide: the other side's eleven is not
// this player's forecast, and filling his row from it would be unreadable.
func TestApplyPerformanceForecast_RefusesARowFromTheWrongSide(t *testing.T) {
	t.Parallel()
	predictor := &fakePerformance{result: &XIPerformanceResult{
		Served:              servedFromRunA,
		InningsMarginalised: true,
		Players: []XIPerformancePlayer{
			{PlayerKey: "a1", Side: Team1Side},
			{PlayerKey: "b1", Side: Team1Side},
		},
	}}

	err := applyPerformanceForecast(context.Background(), predictor, twoSidedFixture("TEST"),
		[]string{"a1"}, []string{"b1"}, resultWithOnePlayerEachSide())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `no forecast for selected player "b1" on side 2`)
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

func TestWinnerFrom_NamesTeam1OnAnExactTie(t *testing.T) {
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
	players := newSelectedPlayers(
		[]string{"k2", "k1"},
		pool(1, 2, 3),
		sideAnswers{Marginals: map[string]float64{"k2": 0.03}},
	)

	require.Len(t, players, 2)
	assert.Equal(t, int64(2), players[0].PlayerID)
	require.NotNil(t, players[0].MarginalValue)
	assert.InDelta(t, 0.03, *players[0].MarginalValue, 1e-9)
	assert.Nil(t, players[1].MarginalValue, "a player with no marginal value carries none")
}

func TestNewSelectedPlayers_DropsAKeyNoPoolRowClaims(t *testing.T) {
	t.Parallel()
	players := newSelectedPlayers([]string{"k1", "ghost"}, pool(1, 2), sideAnswers{})

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
		wantReading      string
	}{
		{
			name:             "unknown draws both batting orders",
			team1BatsFirst:   nil,
			tossMarginalised: true,
			wantReading:      TossReadingMarginalised,
		},
		{
			name:             "team1 bats first",
			team1BatsFirst:   boolValue(true),
			tossMarginalised: false,
			wantReading:      TossReadingAware,
		},
		{
			name:             "team2 bats first",
			team1BatsFirst:   boolValue(false),
			tossMarginalised: false,
			wantReading:      TossReadingAware,
		},
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
			assert.Equal(t, tc.wantReading, result.Toss.Reading, "and names which reading it is")
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

// The response names which of the two quantities its probabilities are, and what in the
// answer did not read the toss (§8.7).
//
// It used to say a named toss "was not used" and blame the format for it. The format was
// never the reason -- `bats_first` orients the per-player row and the batting order is a
// column the display model reads, in TEST as in every other format -- the reason was that
// go-app did not send the field (GO-07).
func TestTossRead_NamesTheReadingAndTheCarveOut(t *testing.T) {
	t.Parallel()

	unknown := tossRead(nil)
	assert.Nil(t, unknown.Team1BatsFirst)
	assert.Equal(t, TossReadingMarginalised, unknown.Reading)
	assert.Empty(t, unknown.Note, "nothing was carved out of an answer that read no toss")

	named := tossRead(boolValue(true))
	require.NotNil(t, named.Team1BatsFirst)
	assert.True(t, *named.Team1BatsFirst)
	assert.Equal(t, TossReadingAware, named.Reading)
	assert.Contains(t, named.Note, "toss-blind objective",
		"the selection and the constraint checks are named as the part that did not read it")
	assert.NotContains(t, named.Note, "no innings length", "the format is not the reason and is not blamed")
}

// The two readings are a declared vocabulary: the Lab labels the card off the value, and
// the ops contract is asserted from both sides (H-24).
func TestTossReadings_AreTheDeclaredVocabulary(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{TossReadingAware, TossReadingMarginalised}, TossReadings())
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

// GO-07: the toss the caller named reaches /performance/predict, and the answer says which
// of the two readings it is.
//
// The performance model orients every player row by `bats_first`, so the toss moves the
// per-player numbers on a format with no innings length exactly as it moves them on one
// with. This path sent nil whatever the caller asked for, on the theory that a format with
// no innings length has no batting order -- so the numbers were marginalised and the
// response blamed the format for it.
func TestApplyPerformanceForecast_CarriesEachTossStateToTheModelAndNamesTheReading(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                string
		team1BatsFirst      *bool
		inningsMarginalised bool
		wantReading         string
		wantNote            bool
	}{
		{
			name:                "unknown averages both batting orders",
			team1BatsFirst:      nil,
			inningsMarginalised: true,
			wantReading:         TossReadingMarginalised,
			wantNote:            false,
		},
		{
			name:                "team1 bats first",
			team1BatsFirst:      boolValue(true),
			inningsMarginalised: false,
			wantReading:         TossReadingAware,
			wantNote:            true,
		},
		{
			name:                "team2 bats first",
			team1BatsFirst:      boolValue(false),
			inningsMarginalised: false,
			wantReading:         TossReadingAware,
			wantNote:            true,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			predictor := &fakePerformance{result: performanceResultForBothSides(tc.inningsMarginalised)}
			fix := twoSidedFixture("TEST")
			fix.team1BatsFirst = tc.team1BatsFirst
			result := resultWithOnePlayerEachSide()

			err := applyPerformanceForecast(
				context.Background(), predictor, fix, []string{"a1"}, []string{"b1"}, result)

			require.NoError(t, err)
			assert.Equal(t, tc.team1BatsFirst, predictor.req.Team1BatsFirst,
				"the toss reaches /performance/predict")
			assert.Equal(t, tc.team1BatsFirst, result.Toss.Team1BatsFirst)
			assert.Equal(t, tc.wantReading, result.Toss.Reading)
			assert.Equal(t, tc.wantNote, result.Toss.Note != "",
				"a toss-aware answer names what in it stayed toss-blind; a marginalised one has nothing to name")
		})
	}
}

// A named toss answered by a marginalised forecast is the request being silently changed,
// and `innings_marginalised` is the one field that can catch it (§8.7, the check the
// simulated path already made for the same reason).
func TestApplyPerformanceForecast_RefusesAForecastThatIgnoredTheTossAsked(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                string
		team1BatsFirst      *bool
		inningsMarginalised bool
		want                string
	}{
		{
			name:                "a named toss answered over both orders",
			team1BatsFirst:      boolValue(true),
			inningsMarginalised: true,
			want:                "the toss was team1 bats first",
		},
		{
			name:                "an unknown toss answered at one order",
			team1BatsFirst:      nil,
			inningsMarginalised: false,
			want:                "the toss was unknown",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			predictor := &fakePerformance{result: performanceResultForBothSides(tc.inningsMarginalised)}
			fix := twoSidedFixture("TEST")
			fix.team1BatsFirst = tc.team1BatsFirst

			err := applyPerformanceForecast(context.Background(), predictor, fix,
				[]string{"a1"}, []string{"b1"}, resultWithOnePlayerEachSide())

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, err.Error(), "innings_marginalised")
		})
	}
}

// performanceResultForBothSides is one forecast row per side, which is what the two
// selected players in resultWithOnePlayerEachSide need.
func performanceResultForBothSides(inningsMarginalised bool) *XIPerformanceResult {
	return &XIPerformanceResult{
		Served:              servedFromRunA,
		InningsMarginalised: inningsMarginalised,
		Players: []XIPerformancePlayer{
			{PlayerKey: "a1", Side: Team1Side, Runs: XISimulatedRange{P10: 3, Median: 26, P90: 71}},
			{PlayerKey: "b1", Side: Team2Side, Runs: XISimulatedRange{P10: 1, Median: 12, P90: 40}},
		},
	}
}

// GO-07: the win call is told the toss too. The displayed probability is the headline on
// every format with no innings length, and until this it was read over both batting orders
// however loudly the caller named one.
func TestNewWinRequest_CarriesTheTossAndTheFixture(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		team1BatsFirst *bool
	}{
		{name: "unknown sends no toss", team1BatsFirst: nil},
		{name: "team1 bats first", team1BatsFirst: boolValue(true)},
		{name: "team2 bats first", team1BatsFirst: boolValue(false)},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fix := twoSidedFixture("TEST")
			fix.team1BatsFirst = tc.team1BatsFirst

			req := newWinRequest(fix, xiSelection{Team1Keys: []string{"a1"}, Team2Keys: []string{"b1"}})

			assert.Equal(t, tc.team1BatsFirst, req.Team1BatsFirst, "the toss reaches /xi/predict-win")
			assert.Equal(t, "TEST", req.Format)
			assert.Equal(t, []string{"a1"}, req.Team1PlayerKeys)
			assert.Nil(t, req.Team1Constraints, "an unpinned eleven asks for no constraint check")
		})
	}
}
