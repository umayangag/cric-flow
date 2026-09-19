package predictteam

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// pinnedFixture is the two-sided fixture with both elevens pinned, which is Play mode:
// the caller has built the sides and only wants them scored (P1-2).
func pinnedFixture(format string, keys1, keys2 []string) fixture {
	fix := twoSidedFixture(format)
	fix.pinned = pinnedXI{team1Keys: keys1, team2Keys: keys2}
	fix.isPinned = true
	return fix
}

func TestChooseXIs_PinnedElevensAreScoredAsSentAndNothingIsSearched(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		format string
	}{
		{name: "an optimised format still scores the caller's eleven", format: "T20I"},
		{name: "a rating-ordered format still scores the caller's eleven", format: "TEST"},
	}
	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}, {"k4", "k5"}}}
			fix := pinnedFixture(testCase.format, []string{"k2", "k3"}, []string{"k5", "k6"})

			selection, err := chooseXIs(context.Background(), optimizer, fix)

			require.NoError(t, err)
			assert.Empty(t, optimizer.calls, "a pinned eleven is not searched for")
			assert.Equal(t, []string{"k2", "k3"}, selection.Team1Keys)
			assert.Equal(t, []string{"k5", "k6"}, selection.Team2Keys)
			assert.Equal(t, SelectionObjectiveFixed, selection.Summary.Objective)
			assert.False(t, selection.Summary.Optimised, "nothing was maximised")
			assert.Contains(t, selection.Summary.Note, "Optimise")
			assert.Nil(t, selection.Team1Answers.Marginals, "no player carries a margin when nothing was searched")
			assert.Nil(t, selection.Team2Answers.Marginals, "no player carries a margin when nothing was searched")
			assert.Equal(t, ServedRatings{}, selection.Served,
				"the stamp comes from the calls that computed the numbers, never from a selection that made none")
		})
	}
}

func TestChooseXIs_WithoutPinnedElevensStillSearches(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}, {"k4", "k5"}}}

	selection, err := chooseXIs(context.Background(), optimizer, twoSidedFixture("TEST"))

	require.NoError(t, err)
	assert.NotEmpty(t, optimizer.calls)
	assert.Equal(t, SelectionObjectiveRatings, selection.Summary.Objective)
}

func TestResolvePinnedXI_ResolvesPlayerIdsToRegistryIdsInTheOrderTheyWereSent(t *testing.T) {
	t.Parallel()

	keys, err := resolvePinnedXI(indiaMen, pool(1, 2, 3), []int64{3, 1}, 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"k3", "k1"}, keys)
}

func TestResolvePinnedXI_RefusesAnElevenThatIsNotOne(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		ids      []int64
		teamSize int
		message  string
	}{
		{name: "short of a full side", ids: []int64{1}, teamSize: 2, message: "1 players pinned"},
		{name: "one too many", ids: []int64{1, 2, 3}, teamSize: 2, message: "3 players pinned"},
	}
	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolvePinnedXI(indiaMen, pool(1, 2, 3), testCase.ids, testCase.teamSize)

			var incomplete *IncompleteXIError
			require.ErrorAs(t, err, &incomplete)
			assert.Contains(t, incomplete.Error(), testCase.message)
			assert.Contains(t, incomplete.Error(), indiaMen.Label())
		})
	}
}

func TestResolvePinnedXI_RefusesAPlayerTheSideCannotField(t *testing.T) {
	t.Parallel()

	_, err := resolvePinnedXI(indiaMen, pool(1, 2, 3), []int64{1, 99}, 2)

	var unknown *UnknownXIPlayerError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, []int64{99}, unknown.PlayerIDs,
		"the id is named rather than dropped: the ten who resolved are not the eleven that was sent")
}

// TestResolvePinnedXI_RefusesTwoIdsThatAreOneRegisteredPlayer: two player rows can carry
// one registry id, and the eleven they make is ten men to every model that reads it
// (SERVE-02). It is refused rather than scored as an eleven.
func TestResolvePinnedXI_RefusesTwoIdsThatAreOneRegisteredPlayer(t *testing.T) {
	t.Parallel()
	rows := []db.PlayerPoolRow{
		{PlayerID: 1, ExternalID: "k1"},
		{PlayerID: 2, ExternalID: "k1", PlayerName: "Imported twice"},
	}

	_, err := resolvePinnedXI(indiaMen, rows, []int64{1, 2}, 2)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "player ids 1 and 2 are the same registered player")
}

func TestResolvePinnedXI_RefusesAPlayerWithNoRegistryId(t *testing.T) {
	t.Parallel()
	rows := []db.PlayerPoolRow{
		{PlayerID: 1, ExternalID: "k1"},
		{PlayerID: 2, ExternalID: "", PlayerName: "Never matched"},
	}

	_, err := resolvePinnedXI(indiaMen, rows, []int64{1, 2}, 2)

	var unknown *UnknownXIPlayerError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, []int64{2}, unknown.PlayerIDs, "ml-service has no rating for a player it was never told about")
}

func TestResolvePinnedXI_RefusesTheSamePlayerTwice(t *testing.T) {
	t.Parallel()

	_, err := resolvePinnedXI(indiaMen, pool(1, 2, 3), []int64{1, 1}, 2)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pinned twice")
}

func TestApplyPinnedXIs_RefusesOneSidePinnedAndTheOtherSearched(t *testing.T) {
	t.Parallel()
	fix := twoSidedFixture("T20I")

	err := applyPinnedXIs(&fix, Input{Team1XI: []int64{1, 2}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "both elevens are pinned or neither is")
	assert.False(t, fix.isPinned)
}

func TestApplyPinnedXIs_WithNeitherSidePinnedLeavesTheFixtureSearched(t *testing.T) {
	t.Parallel()
	fix := twoSidedFixture("T20I")

	err := applyPinnedXIs(&fix, Input{})

	require.NoError(t, err)
	assert.False(t, fix.isPinned)
}

// TestApplyPinnedXIs_CarriesTheFixturesResolvedMustIncludeIds pins the one resolution
// both paths read: Play mode checks the eleven against exactly the ids the search would
// have been locked to, because they are resolved once, on the fixture (B-10).
func TestApplyPinnedXIs_CarriesTheFixturesResolvedMustIncludeIds(t *testing.T) {
	t.Parallel()
	fix := twoSidedFixture("T20I")
	fix.mustInclude1 = []string{"k3"}
	fix.mustInclude2 = []string{"k6"}

	err := applyPinnedXIs(&fix, Input{Team1XI: []int64{1, 2}, Team2XI: []int64{4, 5}})

	require.NoError(t, err)
	assert.True(t, fix.isPinned)
	assert.Equal(t, []string{"k3"}, fix.pinned.mustInclude1)
	assert.Equal(t, []string{"k6"}, fix.pinned.mustInclude2)
}

func TestConstraintCheckRequests_SendTheSameConstraintsTheOptimiserWouldHaveApplied(t *testing.T) {
	t.Parallel()
	fix := pinnedFixture("T20I", []string{"k1", "k2"}, []string{"k4", "k5"})
	fix.pinned.mustInclude1 = []string{"k1"}

	team1, team2 := constraintCheckRequests(fix)

	assert.Equal(t, fix.constraints, team1.Constraints)
	assert.Equal(t, fix.constraints, team2.Constraints)
	assert.Equal(t, []string{"k1"}, team1.MustIncludeKeys)
	assert.Empty(t, team2.MustIncludeKeys)
}

func TestNewConstraintReport_NamesTheMissingMustIncludePlayerRatherThanHisRegistryId(t *testing.T) {
	t.Parallel()
	fix := pinnedFixture("T20I", []string{"k2", "k3"}, []string{"k5", "k6"})
	fix.pool1 = []db.PlayerPoolRow{
		{PlayerID: 1, ExternalID: "k1", PlayerName: "Rohit Sharma"},
		{PlayerID: 2, ExternalID: "k2", PlayerName: "Virat Kohli"},
		{PlayerID: 3, ExternalID: "k3", PlayerName: "Jasprit Bumrah"},
	}
	team1 := &XIConstraintCheck{
		TeamSize: 2, Bowlers: 0, MinBowlers: 1, HasKeeper: false, RequireKeeper: false,
		MissingMustIncludeKeys: []string{"k1"}, Met: false,
	}
	team2 := &XIConstraintCheck{TeamSize: 2, Bowlers: 1, MinBowlers: 1, Met: true}

	report := newConstraintReport(fix, team1, team2)

	assert.Equal(t, fix.constraints.Size, report.TeamSize)
	assert.Equal(t, fix.constraints.MinBowlers, report.MinBowlers)
	assert.False(t, report.Team1.Met)
	assert.Equal(t, 0, report.Team1.Bowlers, "the count is ml-service's, and it is on the wire so the chip can show it")
	require.Len(t, report.Team1.MissingMustInclude, 1)
	assert.Equal(t, MissingPlayer{PlayerID: 1, PlayerName: "Rohit Sharma"}, report.Team1.MissingMustInclude[0])
	assert.True(t, report.Team2.Met)
	assert.Empty(t, report.Team2.MissingMustInclude)
}
