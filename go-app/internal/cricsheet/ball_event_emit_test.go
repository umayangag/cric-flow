package cricsheet_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

// forfeitedAndAllExtrasMatchJSON is the shape of a first-class match the archive actually
// holds, in miniature: four innings, of which the second is forfeited (no overs at all,
// as ten matches in the archive have) and the third is a single no-ball and nothing else
// (as match 514034's fourth innings is -- South Africa needed two and got them off one
// illegal delivery).
//
// Only the third matters to the defect. It has a delivery, so it has cricket in it, but
// no legal ball, so the importer used to skip the whole innings.
const forfeitedAndAllExtrasMatchJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-03-01"],
    "match_type": "Test",
    "team_type": "club",
    "teams": ["Alpha", "Beta"],
    "venue": "The Oval",
    "gender": "male",
    "registry": {"people": {"A1":"a1","A2":"a2","B1":"b1","B2":"b2"}}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":0,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}}
      ]}
    ]},
    {"team":"Beta","forfeited":true},
    {"team":"Alpha","overs":[
      {"over":0,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":1,"extras":1,"total":2},"extras":{"noballs":1}}
      ]}
    ]},
    {"team":"Beta","overs":[
      {"over":0,"deliveries":[
        {"batter":"B2","bowler":"A1","non_striker":"B1","runs":{"batter":0,"extras":0,"total":0}}
      ]}
    ]}
  ]
}`

// TestBuildBallEventRows_AnInningsOfOnlyExtras_KeepsItsDeliveriesAndItsNumber is
// IMPORT-12.
//
// An innings with no legal ball was skipped outright, so the deliveries that were bowled
// in it never reached ball_event while its match_inning row kept its number and its runs.
// The archive holds one such innings and the two runs scored in it were missing from the
// database: `make xi-parity` read 9,345,813 runs from Postgres and 9,345,815 from the
// files. The innings that follow keep the numbers their match_inning rows carry, which is
// what lets the two sources be compared at all.
func TestBuildBallEventRows_AnInningsOfOnlyExtras_KeepsItsDeliveriesAndItsNumber(t *testing.T) {
	t.Parallel()
	// Arrange
	match, err := cricsheet.Parse(strings.NewReader(forfeitedAndAllExtrasMatchJSON))
	require.NoError(t, err)
	identity := playerIDsByName{"A1": 1, "A2": 2, "B1": 11, "B2": 12}

	// Act
	events, err := cricsheet.BuildBallEventRows(
		context.Background(), identity, committedWicketKinds(t), match, 1, 9000012)
	rows := events.Deliveries

	// Assert
	require.NoError(t, err)
	require.Len(t, rows, 3, "the forfeited innings bowled nothing; the other three each bowled one ball")

	innings := make([]int, 0, len(rows))
	for i := range rows {
		innings = append(innings, rows[i].Innings)
	}
	assert.Equal(t, []int{1, 3, 4}, innings,
		"a forfeited second innings is absent, and the innings after it keep their own numbers")

	allExtras := rows[1]
	assert.False(t, allExtras.IsLegal, "a no-ball is not one of the over's balls")
	assert.Equal(t, 0, allExtras.BallSeq, "no legal ball has been bowled in the innings yet")
	assert.Equal(t, 2, allExtras.RunsTotal, "the two runs the innings scored")
	assert.Equal(t, 1, allExtras.ExtrasNoBalls)
	require.NotNil(t, allExtras.StrikerID)
	assert.Equal(t, int64(1), *allExtras.StrikerID, "the batter faced it: a no-ball is faced")
}

// refusingResolver fails for one name and resolves every other, which is how a person the
// database cannot key looks to the row builder.
type refusingResolver struct {
	refuse string
}

func (r refusingResolver) PlayerID(_ context.Context, name string) (int64, error) {
	if name == r.refuse {
		return 0, errors.New("get/create player failed")
	}
	return 7, nil
}

// TestBuildBallEventRows_ANameItCannotResolve_FailsTheFileInsteadOfWritingNULL is
// IMPORT-13.
//
// The striker and non-striker were resolved best-effort: an error left the column NULL
// and was not even logged, so a delivery nobody could be attached to went into ball_event
// looking exactly like one Cricsheet named nobody for. Every other name in the importer
// already fails the file, and this one now does too -- for the dismissed player on the
// wicket row as well.
func TestBuildBallEventRows_ANameItCannotResolve_FailsTheFileInsteadOfWritingNULL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		refuse       string
		wantInsideOf string
	}{
		{name: "the striker", refuse: "A1", wantInsideOf: `resolve striker "A1"`},
		{name: "the non-striker", refuse: "A2", wantInsideOf: `resolve non_striker "A2"`},
		{name: "the bowler", refuse: "B1", wantInsideOf: `resolve bowler "B1"`},
		{name: "the dismissed player", refuse: "B2", wantInsideOf: `resolve player_out "B2"`},
	}
	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			match, err := cricsheet.Parse(strings.NewReader(dismissalMatchJSON))
			require.NoError(t, err)

			// Act
			events, err := cricsheet.BuildBallEventRows(
				context.Background(),
				refusingResolver{refuse: testCase.refuse},
				committedWicketKinds(t),
				match,
				1,
				9000013,
			)

			// Assert
			require.Error(t, err, "a name the database cannot key is a failure, not a NULL")
			assert.ErrorContains(t, err, testCase.wantInsideOf)
			assert.Equal(t, cricsheet.BallEvents{}, events, "nothing is offered for writing")
		})
	}
}

// dismissalMatchJSON is one innings of one over in which B2 is run out, so every role the
// row builder resolves -- striker, non-striker, bowler and dismissed player -- is on the
// same delivery.
const dismissalMatchJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-03-02"],
    "match_type": "T20",
    "team_type": "club",
    "teams": ["Alpha", "Beta"],
    "gender": "male",
    "registry": {"people": {"A1":"a1","A2":"a2","B1":"b1","B2":"b2"}}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":0,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0},
         "wickets":[{"player_out":"B2","kind":"run out"}]}
      ]}
    ]}
  ]
}`

// TestBuildBallEventRows_ANameTheFileLeavesOut_IsStillNobody keeps the other half of the
// rule visible: Cricsheet omits a name it does not have, and no name is NULL rather than
// a failure. Only a resolver that fails is a failure.
func TestBuildBallEventRows_ANameTheFileLeavesOut_IsStillNobody(t *testing.T) {
	t.Parallel()
	// Arrange
	const noNonStriker = `{
  "info": {"balls_per_over": 6, "dates": ["2024-03-03"], "match_type": "T20",
           "team_type": "club", "teams": ["Alpha", "Beta"], "gender": "male"},
  "innings": [{"team":"Alpha","overs":[{"over":0,"deliveries":[
    {"batter":"A1","bowler":"B1","runs":{"batter":1,"extras":0,"total":1}}
  ]}]}]
}`
	match, err := cricsheet.Parse(strings.NewReader(noNonStriker))
	require.NoError(t, err)

	// Act
	events, err := cricsheet.BuildBallEventRows(
		context.Background(), playerIDsByName{"A1": 1, "B1": 11}, committedWicketKinds(t), match, 3, 9000014)

	// Assert
	require.NoError(t, err)
	require.Len(t, events.Deliveries, 1)
	assertNoPlayer(t, events.Deliveries[0].NonStrikerID)
}

func assertNoPlayer(t *testing.T, id *int64) {
	t.Helper()
	assert.Nil(t, id, "a delivery the file names no non-striker for has none")
}
