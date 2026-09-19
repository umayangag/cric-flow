package predictteam

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// namedPool is a pool whose rows carry names, which the card needs to say who the best
// alternative is.
func namedPool(names map[int64]string) []db.PlayerPoolRow {
	rows := make([]db.PlayerPoolRow, 0, len(names))
	for id, name := range names {
		rows = append(rows, db.PlayerPoolRow{PlayerID: id, ExternalID: keyFor(id), PlayerName: name})
	}
	return rows
}

func keyFor(id int64) string {
	return map[int64]string{1: "k1", 2: "k2", 3: "k3"}[id]
}

func TestSelectionRoles_AreTheVocabularyTheCardCanRender(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{RoleKeeper, RoleBowlingOption}, SelectionRoles())
}

// The two source vocabularies the Lab names beside its numbers are the constants the
// prediction path writes, in the order a surface should offer them (H-24, P1-4).
func TestSourceVocabularies_AreTheValuesThePredictionWrites(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{winProbabilitySourceDisplay, winProbabilitySourceSimulator}, WinProbabilitySources())
	assert.Equal(t, []string{forecastSourceSimulator, forecastSourceQuantiles}, ForecastSources())
}

func TestNewSelectionReason_ResolvesTheAlternativeToAPlayerTheClientCanName(t *testing.T) {
	t.Parallel()
	rows := namedPool(map[int64]string{1: "Player A", 2: "Player B", 3: "Player C"})
	byKey := map[string]db.PlayerPoolRow{}
	for _, row := range rows {
		byKey[row.ExternalID] = row
	}

	reason := newSelectionReason(XISelectionReason{
		Roles:              []string{RoleKeeper},
		SelectionRating:    1.5,
		RatingPercentile:   80,
		PoolSize:           3,
		BestAlternativeKey: "k3",
		BestAlternativeGap: 0.012,
	}, byKey)

	require.NotNil(t, reason.BestAlternative)
	assert.Equal(t, int64(3), reason.BestAlternative.PlayerID)
	assert.Equal(t, "Player C", reason.BestAlternative.PlayerName)
	assert.InDelta(t, 0.012, reason.BestAlternative.WinProbabilityGap, 1e-9)
	assert.Empty(t, reason.BestAlternativeNote)
	assert.Equal(t, []string{RoleKeeper}, reason.Roles)
}

// §8.7: a comparison that could not be resolved says so on the wire. Dropping it silently
// would read as "there is no alternative", which is a different claim.
func TestNewSelectionReason_SaysSoWhenThePoolCannotNameTheAlternative(t *testing.T) {
	t.Parallel()
	byKey := map[string]db.PlayerPoolRow{"k1": {PlayerID: 1, ExternalID: "k1", PlayerName: "Player A"}}

	reason := newSelectionReason(XISelectionReason{BestAlternativeKey: "ghost"}, byKey)

	assert.Nil(t, reason.BestAlternative)
	assert.Equal(t, unresolvedAlternativeNote, reason.BestAlternativeNote)
}

func TestNewSelectionReason_KeepsMlServicesOwnReasonForHavingNoAlternative(t *testing.T) {
	t.Parallel()

	reason := newSelectionReason(XISelectionReason{
		BestAlternativeNote: "no swap keeps the eleven inside its constraints",
	}, map[string]db.PlayerPoolRow{})

	assert.Nil(t, reason.BestAlternative)
	assert.Equal(t, "no swap keeps the eleven inside its constraints", reason.BestAlternativeNote)
	assert.Equal(t, []string{}, reason.Roles, "a player answering no constraint has an empty list, not a null")
}

func TestNewSelectedPlayers_AttachesTheSelectionReasonForEveryPlayerThatHasOne(t *testing.T) {
	t.Parallel()
	rows := namedPool(map[int64]string{1: "Player A", 2: "Player B", 3: "Player C"})
	reasons := map[string]XISelectionReason{
		"k1": {
			Roles: []string{RoleKeeper}, SelectionRating: 2, RatingPercentile: 100, PoolSize: 3,
			BestAlternativeKey: "k3", BestAlternativeGap: 0.02,
		},
	}

	players := newSelectedPlayers([]string{"k1", "k2"}, rows, sideAnswers{Reasons: reasons})

	require.Len(t, players, 2)
	require.NotNil(t, players[0].SelectionReason)
	assert.Equal(t, 3, players[0].SelectionReason.PoolSize)
	require.NotNil(t, players[0].SelectionReason.BestAlternative)
	assert.Equal(t, "Player C", players[0].SelectionReason.BestAlternative.PlayerName)
	assert.Nil(t, players[1].SelectionReason, "a player with no reason gets no card, not an empty one")
}

// Play mode never calls the optimiser, so no player carries a reason: the caller built the
// eleven and there is no selection to explain (P1-2, P1-3).
func TestPinnedSelection_CarriesNoSelectionReasons(t *testing.T) {
	t.Parallel()

	selection := pinnedSelection(pinnedXI{team1Keys: []string{"k1"}, team2Keys: []string{"k4"}})

	assert.Nil(t, selection.Team1Answers.Reasons)
	assert.Nil(t, selection.Team2Answers.Reasons)
	assert.Nil(t, selection.Team1Answers.Marginals)
	assert.Nil(t, selection.Team2Answers.Marginals)
}

func TestSelectBothXIs_RatingOrderedSelectionCarriesReasonsWithNoAlternative(t *testing.T) {
	t.Parallel()
	optimizer := &fakeOptimizer{answers: [][]string{{"k1", "k2"}, {"k4", "k5"}}}

	selection, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("TEST"))

	require.NoError(t, err)
	require.Len(t, selection.Team1Answers.Reasons, 2, "team1's reasons are team1's own")
	require.Len(t, selection.Team2Answers.Reasons, 2, "team2's reasons are team2's own")
	for _, answers := range []sideAnswers{selection.Team1Answers, selection.Team2Answers} {
		for key, reason := range answers.Reasons {
			assert.Equal(t, 3, reason.PoolSize, key)
			assert.Empty(t, reason.BestAlternativeKey, "nothing was maximised, so nothing was compared")
		}
	}
}

func TestSelectBothXIs_OptimisedSelectionCarriesTheLastRoundsReasons(t *testing.T) {
	t.Parallel()
	// Two rating-ordered seeds, then a round that changes both XIs and one that settles.
	optimizer := &fakeOptimizer{
		answers: [][]string{{"k1", "k2"}, {"k4", "k5"}, {"k1", "k3"}, {"k4", "k6"}, {"k1", "k3"}, {"k4", "k6"}},
	}

	selection, err := selectBothXIs(context.Background(), optimizer, twoSidedFixture("T20I"))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"k1", "k3"}, keysOf(selection.Team1Answers.Reasons),
		"the reasons describe the eleven that was served, not the seed it started from")
	assert.ElementsMatch(t, []string{"k4", "k6"}, keysOf(selection.Team2Answers.Reasons))
	for _, answers := range []sideAnswers{selection.Team1Answers, selection.Team2Answers} {
		for key, reason := range answers.Reasons {
			assert.NotEmpty(t, reason.BestAlternativeKey, key)
			assert.InDelta(t, 0.005, reason.BestAlternativeGap, 1e-9, key)
		}
	}
}

func keysOf(reasons map[string]XISelectionReason) []string {
	out := make([]string, 0, len(reasons))
	for key := range reasons {
		out = append(out, key)
	}
	return out
}
