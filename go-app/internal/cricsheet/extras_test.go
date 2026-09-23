package cricsheet_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// This file pins IMPORT-04. Cricsheet records a delivery's extras by kind, and a delivery
// can carry two kinds at once -- the archive has 800 no-balls with byes off them and 306
// with leg-byes. The importer used to keep only their sum and one name chosen by
// precedence, and charged the bowler the whole total; the laws charge him wides and
// no-balls and nothing else. Every case below is a shape the archive actually holds.

// extrasMatchJSON is a T20 whose first innings has one of every kind of extra off B1 --
// including the finding's own example, a no-ball with four leg-byes -- and whose second
// innings is an over off A1 in which the only runs are four byes: the bowler's runs are
// zero, so it is a maiden, while the innings total is four.
//
// Innings 1 totals 20 with 15 in extras; B1 concedes 4 + 1 (the no-ball) + 0 + 1 (the
// wide) + 1 (the batter's single beside the penalty) = 7 off 5 legal balls.
const extrasMatchJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-01-02"],
    "match_type": "T20",
    "team_type": "club",
    "teams": ["Alpha", "Beta"],
    "venue": "The Oval",
    "season": "2024",
    "gender": "male",
    "toss": {"winner": "Alpha"},
    "outcome": {"winner": "Alpha"},
    "registry": {"people": {"A1":"a1","A2":"a2","B1":"b1","B2":"b2","B3":"b3"}}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":0,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":5,"total":5},"extras":{"legbyes":4,"noballs":1}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":4,"total":4},"extras":{"byes":4}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":1,"total":1},"extras":{"wides":1}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":1,"extras":5,"total":6},"extras":{"penalty":5}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":0,"extras":0,"total":0}}
      ]}
    ]},
    {"team":"Beta","overs":[
      {"over":0,"deliveries":[
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":0,"extras":4,"total":4},"extras":{"byes":4}},
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":0,"extras":0,"total":0}}
      ]}
    ]}
  ]
}`

func TestParse_ExtrasBreakdown_Decoded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		delivery string
		want     cricsheet.ExtrasBreakdown
	}{
		{
			name:     "a delivery with no extras object decodes to no extras",
			delivery: `{"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}}`,
			want:     cricsheet.ExtrasBreakdown{},
		},
		{
			name:     "a no-ball with four leg-byes keeps both kinds",
			delivery: `{"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":5,"total":5},"extras":{"legbyes":4,"noballs":1}}`,
			want:     cricsheet.ExtrasBreakdown{NoBalls: 1, LegByes: 4},
		},
		{
			name:     "a penalty beside a wide keeps both kinds",
			delivery: `{"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":6,"total":6},"extras":{"penalty":5,"wides":1}}`,
			want:     cricsheet.ExtrasBreakdown{Wides: 1, Penalty: 5},
		},
		{
			// Four deliveries in the archive spell "no leg-byes" this way.
			name:     "a zero count is the same as no count",
			delivery: `{"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0},"extras":{"legbyes":0}}`,
			want:     cricsheet.ExtrasBreakdown{},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			file := `{"info":{"dates":["2024-01-02"],"teams":["Alpha","Beta"]},"innings":[` +
				`{"team":"Alpha","overs":[{"over":0,"deliveries":[` + tc.delivery + `]}]}]}`

			// Act
			match, err := cricsheet.Parse(strings.NewReader(file))

			// Assert
			require.NoError(t, err)
			require.Len(t, match.Innings, 1)
			require.Len(t, match.Innings[0].Overs, 1)
			require.Len(t, match.Innings[0].Overs[0].Deliveries, 1)
			assert.Equal(t, tc.want, match.Innings[0].Overs[0].Deliveries[0].Extras)
		})
	}
}

func TestDelivery_RunsConcededByBowler_LeavesOutByesLegByesAndPenalty(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		delivery cricsheet.Delivery
		want     int
	}{
		{
			name:     "runs off the bat are the bowler's",
			delivery: cricsheet.Delivery{Runs: cricsheet.RunInfo{Batter: 4, Total: 4}},
			want:     4,
		},
		{
			name: "a wide is the bowler's",
			delivery: cricsheet.Delivery{
				Runs:   cricsheet.RunInfo{Extras: 1, Total: 1},
				Extras: cricsheet.ExtrasBreakdown{Wides: 1},
			},
			want: 1,
		},
		{
			name: "a no-ball with four leg-byes charges him the no-ball alone",
			delivery: cricsheet.Delivery{
				Runs:   cricsheet.RunInfo{Extras: 5, Total: 5},
				Extras: cricsheet.ExtrasBreakdown{NoBalls: 1, LegByes: 4},
			},
			want: 1,
		},
		{
			name: "a no-ball the batter hit for two charges him three",
			delivery: cricsheet.Delivery{
				Runs:   cricsheet.RunInfo{Batter: 2, Extras: 1, Total: 3},
				Extras: cricsheet.ExtrasBreakdown{NoBalls: 1},
			},
			want: 3,
		},
		{
			name: "four byes are nobody's",
			delivery: cricsheet.Delivery{
				Runs:   cricsheet.RunInfo{Extras: 4, Total: 4},
				Extras: cricsheet.ExtrasBreakdown{Byes: 4},
			},
			want: 0,
		},
		{
			name: "a penalty beside the batter's single leaves him the single",
			delivery: cricsheet.Delivery{
				Runs:   cricsheet.RunInfo{Batter: 1, Extras: 5, Total: 6},
				Extras: cricsheet.ExtrasBreakdown{Penalty: 5},
			},
			want: 1,
		},
		{
			name: "a leg-bye beside a penalty is nothing to him",
			delivery: cricsheet.Delivery{
				Runs:   cricsheet.RunInfo{Extras: 6, Total: 6},
				Extras: cricsheet.ExtrasBreakdown{LegByes: 1, Penalty: 5},
			},
			want: 0,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.delivery.RunsConcededByBowler()

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildBallEventRows_ExtrasByKind_EveryKindSurvivesTheRow(t *testing.T) {
	t.Parallel()
	// Arrange
	match, err := cricsheet.Parse(strings.NewReader(extrasMatchJSON))
	require.NoError(t, err)
	identity := playerIDsByName{"A1": 1, "A2": 2, "B1": 11, "B2": 12, "B3": 13}

	// Act
	events, err := cricsheet.BuildBallEventRows(
		context.Background(), identity, committedWicketKinds(t), match, 1, 9000011)
	rows := events.Deliveries

	// Assert
	require.NoError(t, err)
	require.Len(t, rows, 13, "the seven deliveries of the first innings and the six of the second")
	testCases := []struct {
		name          string
		row           db.BallEventRow
		wantExtras    [5]int // wides, no-balls, byes, leg-byes, penalty
		wantKind      *string
		wantRunsExtra int
	}{
		{
			name:          "no extras",
			row:           rows[0],
			wantRunsExtra: 0,
		},
		{
			// The finding's example: extras_kind alone reads 'no_ball' and cannot say
			// that four of the five were leg-byes; the breakdown can.
			name:          "a no-ball with four leg-byes",
			row:           rows[1],
			wantExtras:    [5]int{0, 1, 0, 4, 0},
			wantKind:      stringPointer("no_ball"),
			wantRunsExtra: 5,
		},
		{
			name:          "four byes",
			row:           rows[2],
			wantExtras:    [5]int{0, 0, 4, 0, 0},
			wantKind:      stringPointer("bye"),
			wantRunsExtra: 4,
		},
		{
			name:          "a wide",
			row:           rows[3],
			wantExtras:    [5]int{1, 0, 0, 0, 0},
			wantKind:      stringPointer("wide"),
			wantRunsExtra: 1,
		},
		{
			name:          "a penalty",
			row:           rows[4],
			wantExtras:    [5]int{0, 0, 0, 0, 5},
			wantKind:      stringPointer("penalty"),
			wantRunsExtra: 5,
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := [5]int{
				tc.row.ExtrasWides, tc.row.ExtrasNoBalls, tc.row.ExtrasByes,
				tc.row.ExtrasLegByes, tc.row.ExtrasPenalty,
			}
			assert.Equal(t, tc.wantExtras, got, "extras by kind")
			assert.Equal(t, tc.wantKind, tc.row.ExtrasKind, "extras_kind")
			assert.Equal(t, tc.wantRunsExtra, tc.row.RunsExtras, "runs_extras is the sum of the kinds")
		})
	}
}

func stringPointer(value string) *string {
	return &value
}

// bowlingFigure is what the per-file transaction writes to bowling_data for one bowler:
// the two columns the bowler's runs decide, and the one his wickets do.
type bowlingFigure struct {
	runs    int
	maidens int
	wickets int
}

// inningsTotal is what the per-file transaction writes to match_inning for one innings.
type inningsTotal struct {
	runs        int
	extras      int
	wicketsLost int
}

// battingFigure is what the per-file transaction writes to batting_data for one batter:
// the columns a ball faced decides.
type battingFigure struct {
	runs       int
	balls      int
	strikeRate float32
}

// extrasSpyTx captures the bowling figures, batting figures and innings totals the
// per-file transaction writes, keyed by innings number. The fixture bowls one bowler per
// innings, which is what lets the innings number stand in for the bowler: the offline
// import resolves every player to the same id. Batters are more than one per innings and
// cannot be told apart by id, so they are kept as the innings' set of figures.
type extrasSpyTx struct {
	bowlingByInnings map[int]bowlingFigure
	battingByInnings map[int][]battingFigure
	totalsByInnings  map[int]inningsTotal
}

func newExtrasSpyTx() *extrasSpyTx {
	return &extrasSpyTx{
		bowlingByInnings: map[int]bowlingFigure{},
		battingByInnings: map[int][]battingFigure{},
		totalsByInnings:  map[int]inningsTotal{},
	}
}

// Column indexes in the bowling_data_tmp COPY in db.UpsertBowlingBatch (repo_bowling.go)
// and the batting_data_tmp COPY in db.UpsertBattingBatch (repo_batting.go).
const (
	bowlingCopyArgInningNumber = 1
	bowlingCopyArgMaidens      = 5
	bowlingCopyArgRuns         = 6
	bowlingCopyArgWickets      = 7
	battingCopyArgInningNumber = 1
	battingCopyArgRuns         = 4
	battingCopyArgBalls        = 5
	battingCopyArgStrikeRate   = 9
)

func (s *extrasSpyTx) Exec(_ context.Context, sql string, args ...any) error {
	if strings.Contains(sql, "INSERT INTO match_inning") && len(args) >= matchInningArgCount {
		s.totalsByInnings[toInt(args[matchInningArgInningNumber])] = inningsTotal{
			runs:        toInt(args[matchInningArgRunsScored]),
			extras:      toInt(args[matchInningArgExtras]),
			wicketsLost: toInt(args[matchInningArgWicketsLost]),
		}
	}
	return nil
}

func (s *extrasSpyTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}

func (s *extrasSpyTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row { return nopRow{} }

func (s *extrasSpyTx) CopyFrom(
	_ context.Context,
	table pgx.Identifier,
	_ []string,
	src pgx.CopyFromSource,
) (int64, error) {
	isBowling := strings.Join(table, ".") == "bowling_data_tmp"
	isBatting := strings.Join(table, ".") == "batting_data_tmp"
	n := int64(0)
	for src.Next() {
		n++
		values, err := src.Values()
		if err != nil {
			return n, err
		}
		if isBowling && len(values) > bowlingCopyArgWickets {
			s.bowlingByInnings[toInt(values[bowlingCopyArgInningNumber])] = bowlingFigure{
				runs:    *toIntPtr(values[bowlingCopyArgRuns]),
				maidens: *toIntPtr(values[bowlingCopyArgMaidens]),
				wickets: *toIntPtr(values[bowlingCopyArgWickets]),
			}
		}
		if isBatting && len(values) > battingCopyArgStrikeRate {
			innings := toInt(values[battingCopyArgInningNumber])
			s.battingByInnings[innings] = append(s.battingByInnings[innings], battingFigure{
				runs:       *toIntPtr(values[battingCopyArgRuns]),
				balls:      *toIntPtr(values[battingCopyArgBalls]),
				strikeRate: *toFloat32Ptr(values[battingCopyArgStrikeRate]),
			})
		}
	}
	return n, src.Err()
}

func (s *extrasSpyTx) Commit(_ context.Context) error   { return nil }
func (s *extrasSpyTx) Rollback(_ context.Context) error { return nil }

func TestImportMatchFile_ExtrasByKind_ChargesTheBowlerOnlyHisRuns(t *testing.T) {
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
	file := writeTempJSON(t, t.TempDir(), "9000011.json", extrasMatchJSON)

	// Act
	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, inningsTotal{runs: 20, extras: 15}, spy.totalsByInnings[1],
		"the innings keeps every run, byes and leg-byes and penalty included")
	assert.Equal(t, bowlingFigure{runs: 7, maidens: 0}, spy.bowlingByInnings[1],
		"B1 concedes the bat's runs, the no-ball and the wide, not the leg-byes, byes or penalty")
	assert.Equal(t, inningsTotal{runs: 4, extras: 4}, spy.totalsByInnings[2],
		"four byes are four runs to the innings")
	assert.Equal(t, bowlingFigure{runs: 0, maidens: 1}, spy.bowlingByInnings[2],
		"and none to A1, whose over is a maiden")
}
