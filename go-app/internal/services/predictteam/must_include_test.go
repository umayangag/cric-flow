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
