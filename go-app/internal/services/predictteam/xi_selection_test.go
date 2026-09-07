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
	// served is the stamp each call answers with, one per call cycling on the last;
	// empty means every call answers from run A.
	served []ServedRatings
}

// The two rating states a test can be answered from: a prediction is stamped with one, and
// refused when its calls straddle both (P1-5).
var (
	servedFromRunA = ServedRatings{RunID: "20260906T083819Z-36689f80", RatingsThrough: "2026-09-02"}
	servedFromRunB = ServedRatings{RunID: "20260913T083819Z-0e1e39c2", RatingsThrough: "2026-09-09"}
)

// servedOnCall is the stamp for the n-th call of a scripted plan, cycling on the last entry.
func servedOnCall(plan []ServedRatings, call int) ServedRatings {
	if len(plan) == 0 {
		return servedFromRunA
	}
	if call >= len(plan) {
		call = len(plan) - 1
	}
	return plan[call]
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
		SelectionReasons:   scriptedReasons(req, selected),
		Served:             servedOnCall(f.served, len(f.calls)-1),
	}, nil
}

// scriptedReasons answers the way ml-service does: a reason per selected player, with a
// best alternative only where an objective was maximised (P1-3).
func scriptedReasons(req XIOptimizationRequest, selected []string) map[string]XISelectionReason {
	chosen := map[string]bool{}
	for _, key := range selected {
		chosen[key] = true
	}
	alternative := ""
	for _, key := range req.PoolPlayerKeys {
		if !chosen[key] {
			alternative = key
			break
		}
	}
	reasons := map[string]XISelectionReason{}
	for i, key := range selected {
		reason := XISelectionReason{
			Roles:            []string{RoleBowlingOption},
			SelectionRating:  float64(i),
			RatingPercentile: 100 - float64(i)*10,
			PoolSize:         len(req.PoolPlayerKeys),
		}
		if req.Objective == SelectionObjectiveWin {
			reason.BestAlternativeKey = alternative
			reason.BestAlternativeGap = 0.005
		}
		reasons[key] = reason
	}
	return reasons
}

func pool(ids ...int64) []db.PlayerPoolRow {
	rows := make([]db.PlayerPoolRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, db.PlayerPoolRow{PlayerID: id, ExternalID: fmt.Sprintf("k%d", id)})
	}
	return rows
}

// indiaMen and australiaMen are two resolved sides, which is the only thing a fixture holds
// now: a name alone could name either of two teams (D-10).
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

	selection, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("TEST"))

	require.NoError(t, err)
	assert.Equal(t, []string{"k1", "k2"}, selection.Team1Keys)
	assert.Equal(t, []string{"k4", "k5"}, selection.Team2Keys)
	assert.Equal(t, SelectionObjectiveRatings, selection.Summary.Objective)
	assert.False(t, selection.Summary.Optimised)
	assert.Equal(
		t,
		notOptimisedReasons["TEST"],
		selection.Summary.Note,
		"a rating-ordered XI carries its format's reason",
	)
	assert.Contains(t, selection.Summary.Note, "H-17")
	assert.Nil(t, selection.Marginals, "nothing was maximised, so no player has a margin")
	assert.Equal(t, servedFromRunA, selection.Served, "the rating-ordered pick names the state it was read from")
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

	selection, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("T20I"))

	require.NoError(t, err)
	assert.Equal(t, []string{"k1", "k3"}, selection.Team1Keys)
	assert.Equal(t, []string{"k4", "k6"}, selection.Team2Keys)
	assert.Equal(t, SelectionObjectiveWin, selection.Summary.Objective)
	assert.True(t, selection.Summary.Optimised)
	assert.Empty(t, selection.Summary.Note)
	assert.Equal(t, map[string]float64{"k1": 0, "k3": 0.01, "k4": 0, "k6": 0.01}, selection.Marginals,
		"both sides' marginal values reach the response")
	assert.Equal(t, servedFromRunA, selection.Served)

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

	_, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("ODI"))

	require.NoError(t, err)
	assert.Len(t, optimizer.calls, 4, "two seeds and one settled round")
}

// A selection whose optimiser calls were answered from two different runs is refused: the
// XI from one state beside marginals from another has no single date to carry (P1-5).
func TestSelectBothXIs_RefusesASelectionAssembledAcrossAReload(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		format string
		served []ServedRatings
	}{
		{
			name:   "the rating-ordered path sees the second side answered from a new run",
			format: "TEST",
			served: []ServedRatings{servedFromRunA, servedFromRunB},
		},
		{
			name:   "the optimised path sees a round answered from a new run",
			format: "T20I",
			served: []ServedRatings{servedFromRunA, servedFromRunA, servedFromRunA, servedFromRunB},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			optimizer := &fakeOptimizer{
				answers: [][]string{{"k1", "k2"}, {"k4", "k5"}, {"k1", "k2"}, {"k4", "k5"}},
				served:  tc.served,
			}

			_, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture(tc.format))

			var changed *ServedRunChangedError
			require.ErrorAs(t, err, &changed)
			assert.Equal(t, servedFromRunA, changed.Was)
			assert.Equal(t, servedFromRunB, changed.Now)
		})
	}
}

// An optimiser answer that names no run is not an unknown date; it is a dateless
// prediction with a different spelling, and is refused the same way.
func TestSelectBothXIs_RefusesAnAnswerWithNoStamp(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}, {"k4", "k5"}}, served: []ServedRatings{{}}}

	_, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("TEST"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "without naming the run")
}

func TestSelectBothXIs_PropagatesTheOptimiserFailure(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}}, err: errors.New("model not loaded")}

	_, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("T20I"))

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
	assert.Equal(t, "T20I", NormalizeFormat(" t20i "))
	assert.Equal(t, "", NormalizeFormat(""))
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

// TestSelectBothXIs_SendsEachSidesMustIncludeLockOnEveryCall is B-10's fix at the seam it
// broke: go-app used to resolve the must-include ids and then send an empty lock, so the
// search never treated anyone as required. Every call for a side — the rating-ordered
// seed and each best-response round — carries that side's lock and no other side's.
func TestSelectBothXIs_SendsEachSidesMustIncludeLockOnEveryCall(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}, {"k4", "k5"}}}
	fix := twoSidedFixture("T20I")
	fix.mustInclude1 = []string{"k3"}
	fix.mustInclude2 = []string{"k6"}

	_, err := selectBothXIs(context.Background(), optimizer, fix)

	require.NoError(t, err)
	require.NotEmpty(t, optimizer.calls)
	for i := range optimizer.calls {
		call := optimizer.calls[i]
		want := []string{"k6"}
		if call.TeamIsTeam1 {
			want = []string{"k3"}
		}
		assert.Equal(t, want, call.MustIncludeKeys, "call %d", i)
	}
}

// TestSelectBothXIs_WithNoMustIncludeSendsNoLock is the default this change must not move:
// the overwhelming majority of calls ask for nobody in particular, and they must reach
// ml-service exactly as they did before.
func TestSelectBothXIs_WithNoMustIncludeSendsNoLock(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}, {"k4", "k5"}}}

	_, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("T20I"))

	require.NoError(t, err)
	require.NotEmpty(t, optimizer.calls)
	for i := range optimizer.calls {
		assert.Empty(t, optimizer.calls[i].MustIncludeKeys, "call %d", i)
	}
}
