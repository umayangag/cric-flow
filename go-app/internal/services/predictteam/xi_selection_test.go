package predictteam

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// fakeOptimizer records every /xi/optimize call and answers from a scripted plan.
type fakeOptimizer struct {
	calls   []XIOptimizationRequest
	answers [][]int64 // one per call, cycling on the last
	err     error
}

func (f *fakeOptimizer) OptimizeXI(
	_ context.Context,
	req XIOptimizationRequest,
) (*XIOptimizationResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	index := len(f.calls) - 1
	if index >= len(f.answers) {
		index = len(f.answers) - 1
	}
	selected := f.answers[index]
	marginals := map[int64]float64{}
	for i, pid := range selected {
		marginals[pid] = float64(i) / 100
	}
	return &XIOptimizationResult{
		SelectedPlayerIDs: selected,
		Objective:         req.Objective,
		Optimised:         req.Objective == SelectionObjectiveWin,
		MarginalValues:    marginals,
	}, nil
}

func pool(ids ...int64) []db.PlayerPoolRow {
	rows := make([]db.PlayerPoolRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, db.PlayerPoolRow{PlayerID: id})
	}
	return rows
}

func twoSidedFixture(format string) fixture {
	return fixture{
		format:      format,
		team1Code:   "IND",
		team2Code:   "AUS",
		pool1:       pool(1, 2, 3),
		pool2:       pool(4, 5, 6),
		constraints: Constraints{Size: 2, MinBowlers: 1, RequireKeeper: false},
	}
}

func TestSelectBothXIs_TestFormatIsRatingOrderedAndMarkedNotOptimised(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]int64{{1, 2}, {4, 5}}}

	xi1, xi2, summary, marginals, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("TEST"))

	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2}, xi1)
	assert.Equal(t, []int64{4, 5}, xi2)
	assert.Equal(t, SelectionObjectiveRatings, summary.Objective)
	assert.False(t, summary.Optimised)
	assert.NotEmpty(t, summary.Note, "a rating-ordered XI must carry the note every surface shows")
	assert.Nil(t, marginals, "nothing was maximised, so no player has a margin")
	require.Len(t, optimizer.calls, 2, "rating order does not depend on the opponent: one call per side")
	for _, call := range optimizer.calls {
		assert.Equal(t, SelectionObjectiveRatings, call.Objective)
		assert.Empty(t, call.OpponentPlayerIDs)
	}
}

func TestSelectBothXIs_LimitedOversOptimisesAgainstTheOpposingXI(t *testing.T) {
	t.Parallel()
	// Two rating-ordered seeds, then a round that returns the same XIs: a fixed point.
	optimizer := &fakeOptimizer{answers: [][]int64{{1, 2}, {4, 5}, {1, 3}, {4, 6}, {1, 3}, {4, 6}}}

	xi1, xi2, summary, marginals, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("T20"))

	require.NoError(t, err)
	assert.Equal(t, []int64{1, 3}, xi1)
	assert.Equal(t, []int64{4, 6}, xi2)
	assert.Equal(t, SelectionObjectiveWin, summary.Objective)
	assert.True(t, summary.Optimised)
	assert.Empty(t, summary.Note)
	assert.Equal(t, map[int64]float64{1: 0, 3: 0.01, 4: 0, 6: 0.01}, marginals,
		"both sides' marginal values reach the response")

	seeds := optimizer.calls[:2]
	for _, call := range seeds {
		assert.Equal(t, SelectionObjectiveRatings, call.Objective, "the search is seeded by rating order")
	}
	// The first optimised call for team1 plays against team2's seed, not team2's pool.
	assert.Equal(t, SelectionObjectiveWin, optimizer.calls[2].Objective)
	assert.Equal(t, []int64{4, 5}, optimizer.calls[2].OpponentPlayerIDs)
	// team2 then answers team1's *new* XI.
	assert.Equal(t, []int64{1, 3}, optimizer.calls[3].OpponentPlayerIDs)
}

func TestSelectBothXIs_StopsAtAFixedPointRatherThanRunningEveryRound(t *testing.T) {
	t.Parallel()
	// Every call returns the seed, so round one settles.
	optimizer := &fakeOptimizer{answers: [][]int64{{1, 2}, {4, 5}, {1, 2}, {4, 5}}}

	_, _, _, _, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("ODI"))

	require.NoError(t, err)
	assert.Len(t, optimizer.calls, 4, "two seeds and one settled round")
}

func TestSelectBothXIs_PropagatesTheOptimiserFailure(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]int64{{1, 2}}, err: errors.New("model not loaded")}

	_, _, _, _, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("T20"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "model not loaded")
}

func TestOptimizeSide_RefusesAPoolTooSmallForTheConstraints(t *testing.T) {
	t.Parallel()
	fix := twoSidedFixture("T20")
	fix.constraints.Size = 11

	_, err := optimizeSide(context.Background(), &fakeOptimizer{}, fix,
		SelectionObjectiveWin, fix.pool1, []int64{4, 5}, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "need 11")
}

func TestOptimizeSide_RefusesTheWinObjectiveWithoutAnOpposingXI(t *testing.T) {
	t.Parallel()
	fix := twoSidedFixture("T20")

	_, err := optimizeSide(context.Background(), &fakeOptimizer{}, fix,
		SelectionObjectiveWin, fix.pool1, nil, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "opposing XI")
}

func TestSameXI_IgnoresOrderAndCatchesADifference(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		a, b []int64
		want bool
	}{
		{name: "identical", a: []int64{1, 2, 3}, b: []int64{1, 2, 3}, want: true},
		{name: "same players in another order", a: []int64{1, 2, 3}, b: []int64{3, 1, 2}, want: true},
		{name: "one player swapped", a: []int64{1, 2, 3}, b: []int64{1, 2, 4}, want: false},
		{name: "different sizes", a: []int64{1, 2}, b: []int64{1, 2, 3}, want: false},
		{name: "both empty", a: nil, b: nil, want: true},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, sameXI(tc.a, tc.b))
		})
	}
}

func TestNormalizeFormat_FoldsSpacingAndCase(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "T20I", normalizeFormat(" t20i "))
	assert.Equal(t, "", normalizeFormat(""))
}
