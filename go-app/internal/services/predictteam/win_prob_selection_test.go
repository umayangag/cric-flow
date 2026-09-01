package predictteam

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// teamSize is the XI the tests select. Pools are deliberately larger, because the
// defect S-1 fixes is only visible when the pool and the XI differ in size.
const teamSize = 11

// recordingOptimizer implements TeamSelectionOptimizer and records what it was asked.
// It returns the first `teamSize` players of the pool, so the caller's own bookkeeping
// is what the assertions exercise rather than a search.
type recordingOptimizer struct {
	requests []TeamOptimizationRequest
	err      error
	// pick chooses the XI from a request; nil means "the first teamSize players".
	pick func(req TeamOptimizationRequest) []TeamOptSelectedPlayer
}

func (o *recordingOptimizer) PredictMatchWinEnhanced(context.Context, WinFeaturesEnhanced) (float64, error) {
	return 0.5, nil
}

func (o *recordingOptimizer) OptimizeTeamSelection(
	_ context.Context,
	req TeamOptimizationRequest,
) (*TeamOptimizationResult, error) {
	o.requests = append(o.requests, req)
	if o.err != nil {
		return nil, o.err
	}
	selected := o.pickFrom(req)
	return &TeamOptimizationResult{Selected: selected, WinProbability: 0.5}, nil
}

func (o *recordingOptimizer) pickFrom(req TeamOptimizationRequest) []TeamOptSelectedPlayer {
	if o.pick != nil {
		return o.pick(req)
	}
	out := make([]TeamOptSelectedPlayer, 0, teamSize)
	for i := 0; i < teamSize && i < len(req.Pool); i++ {
		out = append(out, TeamOptSelectedPlayer{PlayerID: req.Pool[i].PlayerID, Name: req.Pool[i].Name})
	}
	return out
}

// recordingPredictor implements EnhancedWinPredictor only, so selection falls to the
// per-call optimiser. It records every feature vector it is asked to score.
type recordingPredictor struct {
	seen []WinFeaturesEnhanced
	err  error
}

func (p *recordingPredictor) PredictMatchWinEnhanced(
	_ context.Context,
	features WinFeaturesEnhanced,
) (float64, error) {
	p.seen = append(p.seen, features)
	if p.err != nil {
		return 0, p.err
	}
	return 0.5, nil
}

// buildPools returns matching teamselect and db pools of the given size, plus the
// feature map keyed by player id. Player ids are offset per team so the two sides
// never collide in the feature map.
func buildPools(
	t *testing.T,
	prefix string,
	idOffset int64,
	size int,
) ([]teamselect.Player, []db.PlayerPoolRow, map[int64]map[string]float64) {
	t.Helper()
	tsPool := make([]teamselect.Player, 0, size)
	dbPool := make([]db.PlayerPoolRow, 0, size)
	feats := make(map[int64]map[string]float64, size)
	for i := 0; i < size; i++ {
		name := fmt.Sprintf("%s%02d", prefix, i)
		id := idOffset + int64(i)
		// Every player bowls and keeps, so constraints never decide the XI for us.
		tsPool = append(tsPool, teamselect.Player{
			Name:      name,
			IsBowler:  true,
			IsKeeper:  true,
			BatScore:  float64(size-i) / float64(size),
			BowlScore: float64(size-i) / float64(size),
		})
		dbPool = append(dbPool, db.PlayerPoolRow{PlayerID: id, PlayerName: name})
		feats[id] = map[string]float64{"batting_mean_w5": float64(i), "bowling_std_w10": float64(i)}
	}
	return tsPool, dbPool, feats
}

func mergeFeatures(a, b map[int64]map[string]float64) map[int64]map[string]float64 {
	out := make(map[int64]map[string]float64, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func selectionConstraints() teamselect.Constraints {
	return teamselect.Constraints{Size: teamSize, MinBowlers: 5, RequireKeeper: true}
}

func selectionWeights() teamselect.ScoreWeights {
	return teamselect.DefaultWeights()
}

// TestSelectTeamsByWinProbability_ServerSide_ScoresAgainstOpponentXINotPool is the
// acceptance test for S-1: the opposing side handed to the optimiser must be an XI.
// It used to be the opponent's entire pool, which made every candidate score against a
// match that could not happen.
func TestSelectTeamsByWinProbability_ServerSide_ScoresAgainstOpponentXINotPool(t *testing.T) {
	t.Parallel()

	// Arrange
	poolSize := 18
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, poolSize)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, poolSize)
	optimizer := &recordingOptimizer{}

	// Act
	sel1, sel2, _, err := selectTeamsByWinProbability(
		context.Background(), optimizer, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "T20", 1, 2, 3, 4,
		mergeFeatures(feats1, feats2), time.Time{},
	)

	// Assert
	require.NoError(t, err)
	assert.Len(t, sel1, teamSize)
	assert.Len(t, sel2, teamSize)
	require.NotEmpty(t, optimizer.requests)
	for i := range optimizer.requests {
		assert.Len(t, optimizer.requests[i].OpponentFeatures, teamSize,
			"request %d scored against %d opponents; the opposing side must be an XI, not the pool",
			i, len(optimizer.requests[i].OpponentFeatures))
		assert.Len(t, optimizer.requests[i].Pool, poolSize,
			"the side being optimised still chooses from its whole pool")
	}
}

// TestSelectTeamsByWinProbability_PerCall_ScoresAgainstOpponentXINotPool asserts the
// same property on the fallback path. The two paths must not disagree about what match
// is being scored.
func TestSelectTeamsByWinProbability_PerCall_ScoresAgainstOpponentXINotPool(t *testing.T) {
	t.Parallel()

	// Arrange
	poolSize := 15
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, poolSize)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, poolSize)
	predictor := &recordingPredictor{}

	// Act
	_, _, _, err := selectTeamsByWinProbability(
		context.Background(), predictor, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "T20", 1, 2, 3, 4,
		mergeFeatures(feats1, feats2), time.Time{},
	)

	// Assert
	require.NoError(t, err)
	require.NotEmpty(t, predictor.seen)
	for i := range predictor.seen {
		assert.Len(t, predictor.seen[i].Team1PlayerFeatures, teamSize, "team1 side of evaluation %d", i)
		assert.Len(t, predictor.seen[i].Team2PlayerFeatures, teamSize, "team2 side of evaluation %d", i)
	}
}

// TestSelectTeamsByWinProbability_BothPathsDescribeTheSameFixture: the per-call path
// used to swap the opposition ids when choosing for team2, so the fallback scored a
// different match from the one ml-service scored.
func TestSelectTeamsByWinProbability_BothPathsDescribeTheSameFixture(t *testing.T) {
	t.Parallel()

	// Arrange
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, 12)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, 12)
	predictor := &recordingPredictor{}
	optimizer := &recordingOptimizer{}
	allFeats := mergeFeatures(feats1, feats2)

	// Act
	_, _, _, errPerCall := selectTeamsByWinProbability(
		context.Background(), predictor, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "T20", 1, 2, 3, 4, allFeats, time.Time{},
	)
	_, _, _, errServer := selectTeamsByWinProbability(
		context.Background(), optimizer, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "T20", 1, 2, 3, 4, allFeats, time.Time{},
	)

	// Assert
	require.NoError(t, errPerCall)
	require.NoError(t, errServer)
	require.NotEmpty(t, predictor.seen)
	require.NotEmpty(t, optimizer.requests)
	serverContext := optimizer.requests[0].MatchContext
	for i := range predictor.seen {
		assert.Equal(t, int(serverContext["team1_opposition_id"]), predictor.seen[i].Team1OppositionID,
			"evaluation %d disagrees with the server-side match context", i)
		assert.Equal(t, int(serverContext["team2_opposition_id"]), predictor.seen[i].Team2OppositionID,
			"evaluation %d disagrees with the server-side match context", i)
	}
}

// TestRunBestResponse_StopsAtAFixedPoint: a round that changes neither XI means neither
// side can improve on the other's choice, and further rounds would repeat the work.
func TestRunBestResponse_StopsAtAFixedPoint(t *testing.T) {
	t.Parallel()

	// Arrange
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, 13)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, 13)
	inputs := winProbSelectionInputs{
		pool1:       tsPool1,
		pool2:       tsPool2,
		nameToID1:   buildNameToIDMap(dbPool1),
		nameToID2:   buildNameToIDMap(dbPool2),
		constraints: selectionConstraints(),
		weights:     selectionWeights(),
		format:      "T20",
		allFeats:    mergeFeatures(feats1, feats2),
	}
	calls := 0
	// Always returns the greedy seed, so round 1 changes nothing.
	stable := func(_ context.Context, s selectionSide, _ []teamselect.Player) ([]teamselect.Player, error) {
		calls++
		return teamselect.Select(s.pool, inputs.weights, inputs.constraints)
	}

	// Act
	sel1, sel2, err := runBestResponse(context.Background(), stable, inputs)

	// Assert
	require.NoError(t, err)
	assert.Len(t, sel1, teamSize)
	assert.Len(t, sel2, teamSize)
	assert.Equal(t, 2, calls, "a fixed point in round 1 must stop the loop, not run the cap")
}

// TestRunBestResponse_RunsUpToTheRoundCap: best response can cycle rather than
// converge, so the loop is bounded and returns the last completed round.
func TestRunBestResponse_RunsUpToTheRoundCap(t *testing.T) {
	t.Parallel()

	// Arrange
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, 14)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, 14)
	inputs := winProbSelectionInputs{
		pool1:       tsPool1,
		pool2:       tsPool2,
		nameToID1:   buildNameToIDMap(dbPool1),
		nameToID2:   buildNameToIDMap(dbPool2),
		constraints: selectionConstraints(),
		weights:     selectionWeights(),
		format:      "T20",
		allFeats:    mergeFeatures(feats1, feats2),
	}
	calls := 0
	// Alternates each side between two different XIs on successive rounds, so no round
	// ever settles. Counting per side matters: a counter shared across both sides would
	// hand each side the same XI every round and settle in round two.
	roundsPerSide := map[bool]int{}
	oscillating := func(_ context.Context, s selectionSide, _ []teamselect.Player) ([]teamselect.Player, error) {
		calls++
		roundsPerSide[s.isTeam1]++
		if roundsPerSide[s.isTeam1]%2 == 0 {
			return s.pool[:teamSize], nil
		}
		return s.pool[len(s.pool)-teamSize:], nil
	}

	// Act
	sel1, sel2, err := runBestResponse(context.Background(), oscillating, inputs)

	// Assert
	require.NoError(t, err)
	assert.Len(t, sel1, teamSize)
	assert.Len(t, sel2, teamSize)
	assert.Equal(t, 2*bestResponseRounds(), calls, "the loop must stop at the configured cap")
}

// TestRunBestResponse_ReportsWhichRoundFailed: a mid-loop failure has to say where it
// happened, because "optimize team1" alone does not distinguish a first-round outage
// from a third-round one.
func TestRunBestResponse_ReportsWhichRoundFailed(t *testing.T) {
	t.Parallel()

	// Arrange
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, 12)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, 12)
	inputs := winProbSelectionInputs{
		pool1:       tsPool1,
		pool2:       tsPool2,
		nameToID1:   buildNameToIDMap(dbPool1),
		nameToID2:   buildNameToIDMap(dbPool2),
		constraints: selectionConstraints(),
		weights:     selectionWeights(),
		format:      "T20",
		allFeats:    mergeFeatures(feats1, feats2),
	}
	failing := func(context.Context, selectionSide, []teamselect.Player) ([]teamselect.Player, error) {
		return nil, errors.New("ml-service unreachable")
	}

	// Act
	_, _, err := runBestResponse(context.Background(), failing, inputs)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "round 1")
	assert.Contains(t, err.Error(), "ml-service unreachable")
}

// TestSelectTeamsByWinProbability_FallsBackWhenServerSideFails: the fallback replaces
// the whole selection, not one round, so both XIs come out of the same search.
func TestSelectTeamsByWinProbability_FallsBackWhenServerSideFails(t *testing.T) {
	t.Parallel()

	// Arrange
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, 12)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, 12)
	optimizer := &recordingOptimizer{err: errors.New("optimiser down")}

	// Act
	sel1, sel2, _, err := selectTeamsByWinProbability(
		context.Background(), optimizer, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "T20", 1, 2, 3, 4,
		mergeFeatures(feats1, feats2), time.Time{},
	)

	// Assert
	require.NoError(t, err, "a failing optimiser must fall back, not fail the selection")
	assert.Len(t, sel1, teamSize)
	assert.Len(t, sel2, teamSize)
}

// TestXIFeatures_TakesOnlyTheNamedPlayers guards the helper the fix turns on.
func TestXIFeatures_TakesOnlyTheNamedPlayers(t *testing.T) {
	t.Parallel()

	// Arrange
	tsPool, dbPool, feats := buildPools(t, "a", 1000, 18)
	nameToID := buildNameToIDMap(dbPool)

	// Act
	got := xiFeatures(tsPool[:teamSize], nameToID, feats)

	// Assert
	assert.Len(t, got, teamSize)
	assert.Contains(t, got, int64(1000))
	assert.NotContains(t, got, int64(1000+teamSize), "a player outside the XI must not be scored")
}

// TestSameXI covers the fixed-point test, which order must not affect.
func TestSameXI(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{name: "identical order", a: []string{"x", "y"}, b: []string{"x", "y"}, want: true},
		{name: "same players different order", a: []string{"x", "y"}, b: []string{"y", "x"}, want: true},
		{name: "one player swapped", a: []string{"x", "y"}, b: []string{"x", "z"}, want: false},
		{name: "different sizes", a: []string{"x"}, b: []string{"x", "y"}, want: false},
		{name: "both empty", a: nil, b: nil, want: true},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			toPlayers := func(names []string) []teamselect.Player {
				out := make([]teamselect.Player, 0, len(names))
				for _, n := range names {
					out = append(out, teamselect.Player{Name: n})
				}
				return out
			}
			assert.Equal(t, tc.want, sameXI(toPlayers(tc.a), toPlayers(tc.b)))
		})
	}
}

// TestUsesWinProbabilitySelection: a request-level override exists so one process can
// run both arms of a comparison without writing to the global config and hoping nothing
// else read it in between.
func TestUsesWinProbabilitySelection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		mode         SelectionMode
		configSaysOn bool
		want         bool
	}{
		{name: "default follows config when config is on", mode: SelectionModeDefault, configSaysOn: true, want: true},
		{
			name:         "default follows config when config is off",
			mode:         SelectionModeDefault,
			configSaysOn: false,
			want:         false,
		},
		{
			name:         "winprob overrides a config that is off",
			mode:         SelectionModeWinProbability,
			configSaysOn: false,
			want:         true,
		},
		{name: "greedy overrides a config that is on", mode: SelectionModeGreedy, configSaysOn: true, want: false},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{}
			cfg.Selection.UseWinProbabilitySelection = tc.configSaysOn

			assert.Equal(t, tc.want, usesWinProbabilitySelection(tc.mode, cfg))
		})
	}
}

// TestUsesWinProbabilitySelection_NilConfigIsOff: no config cannot mean "run the hour-long
// search".
func TestUsesWinProbabilitySelection_NilConfigIsOff(t *testing.T) {
	t.Parallel()

	assert.False(t, usesWinProbabilitySelection(SelectionModeDefault, nil))
	assert.True(t, usesWinProbabilitySelection(SelectionModeWinProbability, nil), "an explicit request still wins")
}

// TestIsKnownSelectionMode: a typo must be refused rather than silently falling back to
// the config default, which would quietly run one arm of a comparison twice.
func TestIsKnownSelectionMode(t *testing.T) {
	t.Parallel()

	assert.True(t, IsKnownSelectionMode(SelectionModeDefault))
	assert.True(t, IsKnownSelectionMode(SelectionModeGreedy))
	assert.True(t, IsKnownSelectionMode(SelectionModeWinProbability))
	assert.False(t, IsKnownSelectionMode("win-prob"))
	assert.False(t, IsKnownSelectionMode("GREEDY"))
}
