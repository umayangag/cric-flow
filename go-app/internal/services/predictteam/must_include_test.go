package predictteam

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func mustIncludeFixture(pinned bool) fixture {
	fix := fixture{
		pool1: []db.PlayerPoolRow{
			{PlayerID: 1, ExternalID: "a1", PlayerName: "Picked One"},
			{PlayerID: 2, ExternalID: "a2", PlayerName: "Left Out"},
		},
		pool2: []db.PlayerPoolRow{
			{PlayerID: 3, ExternalID: "b3", PlayerName: "Other Side"},
		},
		isPinned: pinned,
	}
	return fix
}

func TestMustIncludeReport_NamesEveryRequestedPlayerTheSelectionLeftOut(t *testing.T) {
	t.Parallel()
	input := Input{ExtraTeam1: []int64{1, 2}, ExtraTeam2: []int64{3}}
	selection := xiSelection{Team1Keys: []string{"a1"}, Team2Keys: []string{"b3"}}

	report := mustIncludeReport(input, mustIncludeFixture(false), selection)

	require.NotNil(t, report)
	assert.Equal(t, 2, report.Team1.Requested)
	assert.Equal(t, []MissingPlayer{{PlayerID: 2, PlayerName: "Left Out"}}, report.Team1.Missing)
	assert.Equal(t, 1, report.Team2.Requested)
	assert.Empty(t, report.Team2.Missing)
	assert.NotNil(t, report.Team2.Missing, "checked and none missing is an empty list, not an absent one")
}

func TestMustIncludeReport_IsAbsentWhereNothingWasAskedFor(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		input  Input
		pinned bool
	}{
		{name: "no must-include ids on either side", input: Input{}, pinned: false},
		{
			name:   "play mode, where the constraints block carries the check",
			input:  Input{ExtraTeam1: []int64{2}},
			pinned: true,
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			selection := xiSelection{Team1Keys: []string{"a1"}, Team2Keys: []string{"b3"}}

			report := mustIncludeReport(tc.input, mustIncludeFixture(tc.pinned), selection)

			assert.Nil(t, report)
		})
	}
}

func TestMustIncludeKeys_ResolvesTheIdsThisSideCanField(t *testing.T) {
	t.Parallel()
	rows := []db.PlayerPoolRow{
		{PlayerID: 1, ExternalID: "a1", PlayerName: "Picked One"},
		{PlayerID: 2, ExternalID: "a2", PlayerName: "Left Out"},
	}

	keys, err := mustIncludeKeys(indiaMen, rows, []int64{2, 1})

	require.NoError(t, err)
	assert.Equal(t, []string{"a2", "a1"}, keys, "the lock is sent in the order it was asked for")
}

func TestMustIncludeKeys_WithNoIdsAskedForResolvesToAnEmptyLock(t *testing.T) {
	t.Parallel()
	rows := []db.PlayerPoolRow{{PlayerID: 1, ExternalID: "a1", PlayerName: "Picked One"}}

	keys, err := mustIncludeKeys(indiaMen, rows, nil)

	require.NoError(t, err)
	assert.Empty(t, keys, "the overwhelming majority of calls send no lock and must be unchanged by one")
}

// TestMustIncludeKeys_RefusesAnIdThisSideCannotField covers both ways a lock is
// impossible: an id the pool never resolved, and a player the pool holds who carries no
// registry id and so cannot be named to ml-service at all (B-10). Dropping either would
// answer with an eleven that quietly leaves the asked-for player out.
func TestMustIncludeKeys_RefusesAnIdThisSideCannotField(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		rows    []db.PlayerPoolRow
		ids     []int64
		wantIDs []int64
	}{
		{
			name:    "an id no candidate matches",
			rows:    []db.PlayerPoolRow{{PlayerID: 1, ExternalID: "a1"}},
			ids:     []int64{1, 404},
			wantIDs: []int64{404},
		},
		{
			name:    "a candidate with no registry id",
			rows:    []db.PlayerPoolRow{{PlayerID: 1, ExternalID: "a1"}, {PlayerID: 7, ExternalID: ""}},
			ids:     []int64{7},
			wantIDs: []int64{7},
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			keys, err := mustIncludeKeys(indiaMen, tc.rows, tc.ids)

			require.Error(t, err)
			assert.Nil(t, keys)
			var unresolvable *UnresolvableMustIncludeError
			require.ErrorAs(t, err, &unresolvable)
			assert.Equal(t, tc.wantIDs, unresolvable.PlayerIDs)
			assert.Contains(t, unresolvable.Error(), "India (men)")
		})
	}
}

func TestFixtureMustIncludeFor_AnswersPerSide(t *testing.T) {
	t.Parallel()
	fix := fixture{mustInclude1: []string{"a1"}, mustInclude2: []string{"b3"}}

	assert.Equal(t, []string{"a1"}, fix.mustIncludeFor(true))
	assert.Equal(t, []string{"b3"}, fix.mustIncludeFor(false))
}
