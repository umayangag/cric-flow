package cricsheet_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/wicketkinds"
)

// This file pins IMPORT-06. A wicket is one of fourteen kinds and the scorecard does not
// treat them alike: six are the bowler's, six are dismissals nobody bowls, and a batter
// who retires hurt is not out at all. The importer credited every kind to the bowler and
// counted every kind as a wicket lost, and kept only the first wicket of a delivery that
// had two. The fixture is wicketsMatchJSON: one over from B1 in which A1 is caught, A2 is
// run out from the non-striker's end, A4 retires hurt, A5 is bowled while A6 is run out
// on the same ball, and A7 retires out -- six wicket records, five wickets lost, two of
// them B1's.

const wicketsMatchJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-01-03"],
    "match_type": "T20",
    "teams": ["Alpha", "Beta"],
    "venue": "The Oval",
    "season": "2024",
    "gender": "male",
    "toss": {"winner": "Alpha"},
    "outcome": {"winner": "Beta"},
    "players": {"Alpha": ["A1","A2","A3","A4","A5","A6","A7","A8"], "Beta": ["B1","B2","B3"]},
    "registry": {"people": {"A1":"a1","A2":"a2","A3":"a3","A4":"a4","A5":"a5","A6":"a6","A7":"a7","A8":"a8","B1":"b1","B2":"b2","B3":"b3"}}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":0,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0},
         "wickets":[{"player_out":"A1","kind":"caught","fielders":[{"name":"B2"}]}]},
        {"batter":"A3","bowler":"B1","non_striker":"A2","runs":{"batter":1,"extras":0,"total":1},
         "wickets":[{"player_out":"A2","kind":"run out","fielders":[{"name":"B3"}]}]},
        {"batter":"A3","bowler":"B1","non_striker":"A4","runs":{"batter":0,"extras":0,"total":0},
         "wickets":[{"player_out":"A4","kind":"retired hurt"}]},
        {"batter":"A5","bowler":"B1","non_striker":"A6","runs":{"batter":0,"extras":0,"total":0},
         "wickets":[{"player_out":"A5","kind":"bowled"},{"player_out":"A6","kind":"run out","fielders":[{"name":"B2"}]}]},
        {"batter":"A7","bowler":"B1","non_striker":"A3","runs":{"batter":0,"extras":0,"total":0},
         "wickets":[{"player_out":"A7","kind":"retired out"}]},
        {"batter":"A8","bowler":"B1","non_striker":"A3","runs":{"batter":0,"extras":0,"total":0}}
      ]}
    ]}
  ]
}`

// committedWicketKinds is configs/wicket_kinds.json as the importer finds it from this
// directory: the vocabulary the rating pass reads too.
func committedWicketKinds(t *testing.T) wicketkinds.Vocabulary {
	t.Helper()
	vocabulary, err := wicketkinds.LoadFile(filepath.Join("..", "..", "..", "configs", wicketkinds.FileName))
	require.NoError(t, err)
	return vocabulary
}

func wicketsOf(kinds ...string) *cricsheet.Wickets {
	wickets := make(cricsheet.Wickets, 0, len(kinds))
	for _, kind := range kinds {
		wickets = append(wickets, cricsheet.Wicket{PlayerOut: "X", Kind: kind})
	}
	return &wickets
}

func TestDelivery_TallyWickets_CreditsTheBowlerTheWayTheScorecardDoes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		wickets *cricsheet.Wickets
		want    cricsheet.WicketTally
	}{
		{
			name:    "no wicket",
			wickets: nil,
			want:    cricsheet.WicketTally{},
		},
		{
			name:    "a catch is the bowler's",
			wickets: wicketsOf("caught"),
			want:    cricsheet.WicketTally{Dismissals: 1, CreditedToBowler: 1},
		},
		{
			name:    "a run out is a wicket lost and not the bowler's",
			wickets: wicketsOf("run out"),
			want:    cricsheet.WicketTally{Dismissals: 1, CreditedToBowler: 0},
		},
		{
			name:    "a retired out is a wicket lost and not the bowler's",
			wickets: wicketsOf("retired out"),
			want:    cricsheet.WicketTally{Dismissals: 1, CreditedToBowler: 0},
		},
		{
			name:    "a retired hurt is not a wicket lost",
			wickets: wicketsOf("retired hurt"),
			want:    cricsheet.WicketTally{},
		},
		{
			name:    "a retired not out is not a wicket lost",
			wickets: wicketsOf("retired not out"),
			want:    cricsheet.WicketTally{},
		},
		{
			name:    "a stumping is the bowler's",
			wickets: wicketsOf("stumped"),
			want:    cricsheet.WicketTally{Dismissals: 1, CreditedToBowler: 1},
		},
		{
			name:    "obstructing the field is nobody's",
			wickets: wicketsOf("obstructing the field"),
			want:    cricsheet.WicketTally{Dismissals: 1, CreditedToBowler: 0},
		},
		{
			name:    "bowled with the non-striker run out is two wickets lost and one of the bowler's",
			wickets: wicketsOf("bowled", "run out"),
			want:    cricsheet.WicketTally{Dismissals: 2, CreditedToBowler: 1},
		},
		{
			name:    "caught with the non-striker retired hurt is one wicket lost",
			wickets: wicketsOf("caught", "retired hurt"),
			want:    cricsheet.WicketTally{Dismissals: 1, CreditedToBowler: 1},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			delivery := cricsheet.Delivery{Wickets: tc.wickets}

			got, err := delivery.TallyWickets(committedWicketKinds(t))

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDelivery_TallyWickets_UnknownKindIsAnError(t *testing.T) {
	t.Parallel()
	delivery := cricsheet.Delivery{Wickets: wicketsOf("mankaded")}

	_, err := delivery.TallyWickets(committedWicketKinds(t))

	require.Error(t, err)
}

func TestBuildBallEventRows_EveryWicketOnADeliveryIsARow(t *testing.T) {
	t.Parallel()
	// Arrange
	match, err := cricsheet.Parse(strings.NewReader(wicketsMatchJSON))
	require.NoError(t, err)
	identity := playerIDsByName{
		"A1": 1, "A2": 2, "A3": 3, "A4": 4, "A5": 5, "A6": 6, "A7": 7, "A8": 8, "B1": 11, "B2": 12, "B3": 13,
	}

	// Act
	events, err := cricsheet.BuildBallEventRows(
		context.Background(), identity, committedWicketKinds(t), match, 1, 9000012)

	// Assert
	require.NoError(t, err)
	require.Len(t, events.Deliveries, 6)
	wicket := func(ball, number int, kind string, playerOut int64) db.BallEventWicketRow {
		return db.BallEventWicketRow{
			MatchID: 9000012, Innings: 1, Over: 0, Ball: ball, WicketNumber: number, Kind: kind, PlayerOutID: &playerOut,
		}
	}
	assert.Equal(t, []db.BallEventWicketRow{
		wicket(1, 1, "caught", 1),
		wicket(2, 1, "run out", 2),
		wicket(3, 1, "retired hurt", 4),
		wicket(4, 1, "bowled", 5),
		wicket(4, 2, "run out", 6),
		wicket(5, 1, "retired out", 7),
	}, events.Wickets, "six wicket records, the fourth ball's two both present and in the file's order")
}

func TestBuildBallEventRows_UnknownWicketKindFailsTheFile(t *testing.T) {
	t.Parallel()
	// Arrange
	unknown := strings.Replace(wicketsMatchJSON, `"kind":"retired out"`, `"kind":"mankaded"`, 1)
	match, err := cricsheet.Parse(strings.NewReader(unknown))
	require.NoError(t, err)
	identity := playerIDsByName{}

	// Act
	_, err = cricsheet.BuildBallEventRows(
		context.Background(), identity, committedWicketKinds(t), match, 1, 9000012)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"mankaded"`)
}

func TestImportMatchFile_Wickets_TheScorecardsCountsNotTheFiles(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	// Arrange
	ctx := context.Background()
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	spy := newExtrasSpyTx()
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spy)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
	file := writeTempJSON(t, t.TempDir(), "9000012.json", wicketsMatchJSON)

	// Act
	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, bowlingFigure{runs: 1, maidens: 0, wickets: 2}, spy.bowlingByInnings[1],
		"B1 is credited with the catch and the bowled, not the run outs or the retirements")
	assert.Equal(t, inningsTotal{runs: 1, extras: 0, wicketsLost: 5}, spy.totalsByInnings[1],
		"five wickets lost: the retired-hurt batter is not out")
}
