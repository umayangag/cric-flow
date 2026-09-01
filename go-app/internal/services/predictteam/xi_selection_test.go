package predictteam

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// recordingXIOptimizer implements EnhancedWinPredictor and XISelectionOptimizer and records
// every /xi/optimize request. It picks the first teamSize ids of the pool.
type recordingXIOptimizer struct {
	requests []XIOptimizationRequest
	err      error
}

func (o *recordingXIOptimizer) PredictMatchWinEnhanced(context.Context, WinFeaturesEnhanced) (float64, error) {
	return 0.5, nil
}

func (o *recordingXIOptimizer) OptimizeXI(_ context.Context, req XIOptimizationRequest) (*XIOptimizationResult, error) {
	o.requests = append(o.requests, req)
	if o.err != nil {
		return nil, o.err
	}
	n := teamSize
	if len(req.PoolPlayerIDs) < n {
		n = len(req.PoolPlayerIDs)
	}
	return &XIOptimizationResult{
		SelectedPlayerIDs: append([]int64(nil), req.PoolPlayerIDs[:n]...),
		WinProbability:    0.6,
	}, nil
}

func withXIWinModel(t *testing.T, enabled bool) {
	t.Helper()
	previous := selectionUsesXIWinModel
	selectionUsesXIWinModel = func() bool { return enabled }
	t.Cleanup(func() { selectionUsesXIWinModel = previous })
}

func TestSelectTeamsByWinProbability_XIModel_SendsIDsNotFeatures(t *testing.T) {
	// Arrange
	withXIWinModel(t, true)
	poolSize := 18
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, poolSize)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, poolSize)
	optimizer := &recordingXIOptimizer{}

	// Act
	sel1, sel2, err := selectTeamsByWinProbability(
		context.Background(), optimizer, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "t20", 1, 2, 3, 4,
		mergeFeatures(feats1, feats2), time.Time{},
	)

	// Assert
	require.NoError(t, err)
	assert.Len(t, sel1, teamSize)
	assert.Len(t, sel2, teamSize)
	require.NotEmpty(t, optimizer.requests)
	for i := range optimizer.requests {
		req := optimizer.requests[i]
		assert.Equal(t, "T20", req.Format, "format is normalised to upper case")
		assert.Len(t, req.PoolPlayerIDs, poolSize, "the side being optimised chooses from its whole pool")
		assert.Len(t, req.OpponentPlayerIDs, teamSize, "the opposing side is an XI, never the pool (S-1)")
		assert.Equal(t, selectionConstraints(), req.Constraints)
	}
	assert.True(t, optimizer.requests[0].TeamIsTeam1, "team1 is optimised first")
	assert.False(t, optimizer.requests[1].TeamIsTeam1, "then team2 against team1's new XI")
}

func TestSelectTeamsByWinProbability_XIModel_FallsBackWhenUnavailable(t *testing.T) {
	// Arrange
	withXIWinModel(t, true)
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, 14)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, 14)
	optimizer := &recordingXIOptimizer{err: errors.New("xi artifacts not loaded")}

	// Act
	sel1, sel2, err := selectTeamsByWinProbability(
		context.Background(), optimizer, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "T20", 1, 2, 3, 4,
		mergeFeatures(feats1, feats2), time.Time{},
	)

	// Assert: the per-call windowed-form path still produces both XIs
	require.NoError(t, err)
	assert.Len(t, sel1, teamSize)
	assert.Len(t, sel2, teamSize)
	assert.NotEmpty(t, optimizer.requests, "the XI path was attempted before falling back")
}

func TestSelectTeamsByWinProbability_XIModel_IgnoredWhenConfigSaysWindowedForm(t *testing.T) {
	// Arrange
	withXIWinModel(t, false)
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, 14)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, 14)
	optimizer := &recordingXIOptimizer{}

	// Act
	_, _, err := selectTeamsByWinProbability(
		context.Background(), optimizer, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "T20", 1, 2, 3, 4,
		mergeFeatures(feats1, feats2), time.Time{},
	)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, optimizer.requests, "selection.win_model != xi must not call /xi/optimize")
}

func TestPoolPlayerIDs_SkipsUnknownAndDuplicateNames(t *testing.T) {
	testCases := []struct {
		name     string
		pool     []teamselect.Player
		nameToID map[string]int64
		wantIDs  []int64
	}{
		{
			name:     "unknown names are skipped",
			pool:     []teamselect.Player{{Name: "a"}, {Name: "b"}, {Name: "c"}},
			nameToID: map[string]int64{"a": 1, "c": 3},
			wantIDs:  []int64{1, 3},
		},
		{
			name:     "two names on one id keep the first",
			pool:     []teamselect.Player{{Name: "a"}, {Name: "a2"}},
			nameToID: map[string]int64{"a": 1, "a2": 1},
			wantIDs:  []int64{1},
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			ids, byID := poolPlayerIDs(tc.pool, tc.nameToID)
			assert.Equal(t, tc.wantIDs, ids)
			assert.Len(t, byID, len(tc.wantIDs))
		})
	}
}

func TestXIResultToPlayers_ReturnsPoolPlayersSortedByName(t *testing.T) {
	byID := map[int64]teamselect.Player{1: {Name: "zed", BatScore: 1}, 2: {Name: "amy", BatScore: 2}}
	result := &XIOptimizationResult{SelectedPlayerIDs: []int64{1, 2, 99}}

	out := xiResultToPlayers(result, byID)

	require.Len(t, out, 2, "an id the pool does not know is dropped, not invented")
	assert.Equal(t, "amy", out[0].Name)
	assert.Equal(t, "zed", out[1].Name)
	assert.Equal(t, 1.0, out[1].BatScore, "the pool's own player record is returned, scores intact")
}

func TestSelectTeamsByWinProbability_XIModel_CarriesAsOfToEveryOptimizeCall(t *testing.T) {
	// Arrange: a backtest asks for ratings as they stood before the match date.
	withXIWinModel(t, true)
	tsPool1, dbPool1, feats1 := buildPools(t, "a", 1000, 14)
	tsPool2, dbPool2, feats2 := buildPools(t, "b", 2000, 14)
	optimizer := &recordingXIOptimizer{}
	asOf := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)

	// Act
	_, _, err := selectTeamsByWinProbability(
		context.Background(), optimizer, tsPool1, tsPool2, selectionConstraints(),
		dbPool1, dbPool2, selectionWeights(), "T20", 1, 2, 3, 4,
		mergeFeatures(feats1, feats2), asOf,
	)

	// Assert
	require.NoError(t, err)
	require.NotEmpty(t, optimizer.requests)
	for i := range optimizer.requests {
		assert.Equal(t, asOf, optimizer.requests[i].AsOf,
			"both sides' searches must run against the same as-of state")
	}
}
