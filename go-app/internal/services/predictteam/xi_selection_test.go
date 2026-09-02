package predictteam

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/teams"
)

// fakeOptimizer records every /xi/optimize call and answers from a scripted plan.
type fakeOptimizer struct {
	calls   []XIOptimizationRequest
	answers [][]string // one per call, cycling on the last
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
	marginals := map[string]float64{}
	for i, key := range selected {
		marginals[key] = float64(i) / 100
	}
	return &XIOptimizationResult{
		SelectedPlayerKeys: selected,
		Objective:          req.Objective,
		Optimised:          req.Objective == SelectionObjectiveWin,
		MarginalValues:     marginals,
	}, nil
}

func pool(ids ...int64) []db.PlayerPoolRow {
	rows := make([]db.PlayerPoolRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, db.PlayerPoolRow{PlayerID: id, ExternalID: fmt.Sprintf("k%d", id)})
	}
	return rows
}

// indiaMen and australiaMen are two resolved sides, which is the only thing a fixture holds
// now: a name alone could name either of two teams (D-11).
var (
	indiaMen     = db.TeamSide{ClubID: 43, Name: "India", Gender: teams.GenderMale}
	australiaMen = db.TeamSide{ClubID: 7, Name: "Australia", Gender: teams.GenderMale}
)

func twoSidedFixture(format string) fixture {
	return fixture{
		format:      format,
		team1:       indiaMen,
		team2:       australiaMen,
		pool1:       pool(1, 2, 3),
		pool2:       pool(4, 5, 6),
		constraints: Constraints{Size: 2, MinBowlers: 1, RequireKeeper: false},
	}
}

func TestSelectBothXIs_TestFormatIsRatingOrderedAndMarkedNotOptimised(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}, {"k4", "k5"}}}

	xi1, xi2, summary, marginals, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("TEST"))

	require.NoError(t, err)
	assert.Equal(t, []string{"k1", "k2"}, xi1)
	assert.Equal(t, []string{"k4", "k5"}, xi2)
	assert.Equal(t, SelectionObjectiveRatings, summary.Objective)
	assert.False(t, summary.Optimised)
	assert.Equal(t, notOptimisedReasons["TEST"], summary.Note, "a rating-ordered XI carries its format's reason")
	assert.Contains(t, summary.Note, "H-17")
	assert.Nil(t, marginals, "nothing was maximised, so no player has a margin")
	require.Len(t, optimizer.calls, 2, "rating order does not depend on the opponent: one call per side")
	for _, call := range optimizer.calls {
		assert.Equal(t, SelectionObjectiveRatings, call.Objective)
		assert.Empty(t, call.OpponentPlayerKeys)
	}
}

func TestIsOptimisedSelectionFormat_FollowsTheReasonsMapBothWays(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		format    string
		optimised bool
	}{
		{name: "T20 is rating-ordered under E5", format: "T20", optimised: false},
		{name: "T20I is searched on the win objective", format: "T20I", optimised: true},
		{name: "ODI is searched on the win objective", format: "ODI", optimised: true},
		{name: "TEST is rating-ordered under H-17", format: "TEST", optimised: false},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.optimised, isOptimisedSelectionFormat(tc.format))
			_, hasReason := notOptimisedReasons[tc.format]
			assert.Equal(t, !tc.optimised, hasReason, "every format that is not optimised names its reason")
		})
	}
}

func TestNotOptimisedReasons_EveryReasonNamesTheRule(t *testing.T) {
	t.Parallel()
	for format, reason := range notOptimisedReasons {
		assert.Contains(t, reason, "Rating-ordered XI", format)
		assert.True(t, strings.Contains(reason, "H-17") || strings.Contains(reason, "E5"), "%s: %s", format, reason)
	}
}

func TestSelectBothXIs_LimitedOversOptimisesAgainstTheOpposingXI(t *testing.T) {
	t.Parallel()
	// Two rating-ordered seeds, then a round that returns the same XIs: a fixed point.
	optimizer := &fakeOptimizer{
		answers: [][]string{{"k1", "k2"}, {"k4", "k5"}, {"k1", "k3"}, {"k4", "k6"}, {"k1", "k3"}, {"k4", "k6"}},
	}

	xi1, xi2, summary, marginals, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("T20I"))

	require.NoError(t, err)
	assert.Equal(t, []string{"k1", "k3"}, xi1)
	assert.Equal(t, []string{"k4", "k6"}, xi2)
	assert.Equal(t, SelectionObjectiveWin, summary.Objective)
	assert.True(t, summary.Optimised)
	assert.Empty(t, summary.Note)
	assert.Equal(t, map[string]float64{"k1": 0, "k3": 0.01, "k4": 0, "k6": 0.01}, marginals,
		"both sides' marginal values reach the response")

	seeds := optimizer.calls[:2]
	for _, call := range seeds {
		assert.Equal(t, SelectionObjectiveRatings, call.Objective, "the search is seeded by rating order")
	}
	// The first optimised call for team1 plays against team2's seed, not team2's pool.
	assert.Equal(t, SelectionObjectiveWin, optimizer.calls[2].Objective)
	assert.Equal(t, []string{"k4", "k5"}, optimizer.calls[2].OpponentPlayerKeys)
	// team2 then answers team1's *new* XI.
	assert.Equal(t, []string{"k1", "k3"}, optimizer.calls[3].OpponentPlayerKeys)
}

func TestSelectBothXIs_StopsAtAFixedPointRatherThanRunningEveryRound(t *testing.T) {
	t.Parallel()
	// Every call returns the seed, so round one settles.
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}, {"k4", "k5"}, {"k1", "k2"}, {"k4", "k5"}}}

	_, _, _, _, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("ODI"))

	require.NoError(t, err)
	assert.Len(t, optimizer.calls, 4, "two seeds and one settled round")
}

func TestSelectBothXIs_PropagatesTheOptimiserFailure(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}}, err: errors.New("model not loaded")}

	_, _, _, _, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("T20I"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "model not loaded")
}

func TestOptimizeSide_RefusesAPoolTooSmallForTheConstraints(t *testing.T) {
	t.Parallel()
	fix := twoSidedFixture("T20I")
	fix.constraints.Size = 11

	_, err := optimizeSide(context.Background(), &fakeOptimizer{}, fix,
		SelectionObjectiveWin, fix.pool1, []string{"k4", "k5"}, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "need 11")
}

func TestOptimizeSide_RefusesTheWinObjectiveWithoutAnOpposingXI(t *testing.T) {
	t.Parallel()
	fix := twoSidedFixture("T20I")

	_, err := optimizeSide(context.Background(), &fakeOptimizer{}, fix,
		SelectionObjectiveWin, fix.pool1, nil, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "opposing XI")
}

func TestSameXI_IgnoresOrderAndCatchesADifference(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		a, b []string
		want bool
	}{
		{name: "identical", a: []string{"a", "b", "c"}, b: []string{"a", "b", "c"}, want: true},
		{name: "same players in another order", a: []string{"a", "b", "c"}, b: []string{"c", "a", "b"}, want: true},
		{name: "one player swapped", a: []string{"a", "b", "c"}, b: []string{"a", "b", "d"}, want: false},
		{name: "different sizes", a: []string{"a", "b"}, b: []string{"a", "b", "c"}, want: false},
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

func TestPoolPlayerKeys_SendsRegistryIDsAndSkipsAPlayerWithout(t *testing.T) {
	t.Parallel()
	rows := []db.PlayerPoolRow{
		{PlayerID: 1, ExternalID: "2911de16"},
		{PlayerID: 2, PlayerName: "unmatched"},
		{PlayerID: 3, ExternalID: "a8c9f0b1"},
	}

	keys := poolPlayerKeys(rows)

	assert.Equal(t, []string{"2911de16", "a8c9f0b1"}, keys,
		"the wire carries registry ids; a player the importer never matched cannot be selected")
}
