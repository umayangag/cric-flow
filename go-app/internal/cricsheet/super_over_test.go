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

// This file pins IMPORT-01: a super over is a tie-breaker Cricsheet appends to the innings
// list, and the importer used to write it as innings 3 and 4 of the match -- career balls,
// runs, dismissals and a batting position for whoever played it. The innings record must
// hold the innings of the match and nothing else, on both write paths.

// superOverTieJSON is a T20 tie decided by a super over. B7 bats and A4 bowls only in the
// super over, so any row either of them earns is a row the tie-breaker put there. The
// third innings is deliberately a super over *before* the fourth so the numbering, not
// just the count, is under test.
const superOverTieJSON = `{
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
    "outcome": {"winner": "Beta", "result": "tie", "eliminator": "Beta"},
    "registry": {"people": {"A1":"a1","A2":"a2","A4":"a4","B1":"b1","B2":"b2","B3":"b3","B7":"b7"}}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":0,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":1,"total":1},"extras":{"wides":1}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":6,"extras":0,"total":6}}
      ]}
    ]},
    {"team":"Beta","overs":[
      {"over":0,"deliveries":[
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":4,"extras":0,"total":4}},
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":6,"extras":0,"total":6}},
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":1,"extras":0,"total":1}}
      ]}
    ]},
    {"team":"Beta","super_over":true,"overs":[
      {"over":0,"deliveries":[
        {"batter":"B7","bowler":"A4","non_striker":"B2","runs":{"batter":6,"extras":0,"total":6}},
        {"batter":"B7","bowler":"A4","non_striker":"B2","runs":{"batter":0,"extras":0,"total":0},"wickets":[{"player_out":"B7","kind":"bowled"}]}
      ]}
    ]},
    {"team":"Alpha","super_over":true,"overs":[
      {"over":0,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":1,"extras":0,"total":1}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":0,"extras":0,"total":0},"wickets":[{"player_out":"A2","kind":"lbw"}]}
      ]}
    ]}
  ]
}`

func TestParse_InningsFlags_Decoded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		innings string
		assert  func(t *testing.T, innings cricsheet.Innings)
	}{
		{
			name:    "super_over is read",
			innings: `{"team":"Alpha","super_over":true,"overs":[]}`,
			assert: func(t *testing.T, innings cricsheet.Innings) {
				assert.True(t, innings.SuperOver)
				assert.False(t, innings.Declared)
				assert.False(t, innings.Forfeited)
				assert.Nil(t, innings.Target)
			},
		},
		{
			name:    "declared is read",
			innings: `{"team":"Alpha","declared":true,"overs":[]}`,
			assert: func(t *testing.T, innings cricsheet.Innings) {
				assert.True(t, innings.Declared)
				assert.False(t, innings.SuperOver)
			},
		},
		{
			name:    "a forfeited innings has no overs and still parses",
			innings: `{"team":"Alpha","forfeited":true}`,
			assert: func(t *testing.T, innings cricsheet.Innings) {
				assert.True(t, innings.Forfeited)
				assert.Empty(t, innings.Overs)
			},
		},
		{
			name:    "a whole-overs target is read",
			innings: `{"team":"Beta","target":{"overs":20,"runs":181},"overs":[]}`,
			assert: func(t *testing.T, innings cricsheet.Innings) {
				require.NotNil(t, innings.Target)
				assert.InDelta(t, 20.0, innings.Target.Overs, 0)
				assert.Equal(t, 181, innings.Target.Runs)
			},
		},
		{
			name: "a rain-revised target in overs-and-balls notation is read, not refused",
			// 158 innings in the archive carry a float here; an int field would fail the file.
			innings: `{"team":"Beta","target":{"overs":12.4,"runs":97},"overs":[]}`,
			assert: func(t *testing.T, innings cricsheet.Innings) {
				require.NotNil(t, innings.Target)
				assert.InDelta(t, 12.4, innings.Target.Overs, 1e-9)
				assert.Equal(t, 97, innings.Target.Runs)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			file := `{"info":{"dates":["2024-01-02"],"teams":["Alpha","Beta"]},"innings":[` + tc.innings + `]}`

			// Act
			match, err := cricsheet.Parse(strings.NewReader(file))

			// Assert
			require.NoError(t, err)
			require.Len(t, match.Innings, 1)
			tc.assert(t, match.Innings[0])
		})
	}
}

func TestPlayedInnings_LeavesOutSuperOvers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		innings   []cricsheet.Innings
		wantTeams []string
	}{
		{
			name:      "a match with no super over keeps every innings in order",
			innings:   []cricsheet.Innings{{Team: "Alpha"}, {Team: "Beta"}},
			wantTeams: []string{"Alpha", "Beta"},
		},
		{
			name: "a tie decided by one super over keeps the two innings of the match",
			innings: []cricsheet.Innings{
				{Team: "Alpha"},
				{Team: "Beta"},
				{Team: "Beta", SuperOver: true},
				{Team: "Alpha", SuperOver: true},
			},
			wantTeams: []string{"Alpha", "Beta"},
		},
		{
			name: "a tie that needed two super overs still keeps only the two innings of the match",
			innings: []cricsheet.Innings{
				{Team: "Alpha"},
				{Team: "Beta"},
				{Team: "Beta", SuperOver: true},
				{Team: "Alpha", SuperOver: true},
				{Team: "Alpha", SuperOver: true},
				{Team: "Beta", SuperOver: true},
			},
			wantTeams: []string{"Alpha", "Beta"},
		},
		{
			name: "a first-class match's four innings are all played innings, declared or not",
			innings: []cricsheet.Innings{
				{Team: "Alpha", Declared: true},
				{Team: "Beta"},
				{Team: "Alpha", Forfeited: true},
				{Team: "Beta"},
			},
			wantTeams: []string{"Alpha", "Beta", "Alpha", "Beta"},
		},
		{
			name:      "no innings at all",
			innings:   nil,
			wantTeams: []string{},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			match := &cricsheet.Match{Innings: tc.innings}

			// Act
			played := match.PlayedInnings()

			// Assert
			teams := make([]string, 0, len(played))
			for i := range played {
				teams = append(teams, played[i].Team)
			}
			assert.Equal(t, tc.wantTeams, teams)
		})
	}
}

// playerIDsByName resolves delivery names for BuildBallEventRows without a database.
type playerIDsByName map[string]int64

func (p playerIDsByName) PlayerID(_ context.Context, name string) (int64, error) {
	return p[name], nil
}

func TestBuildBallEventRows_SuperOverFile_WritesNoInningsAboveTwo(t *testing.T) {
	t.Parallel()
	// Arrange
	match, err := cricsheet.Parse(strings.NewReader(superOverTieJSON))
	require.NoError(t, err)
	identity := playerIDsByName{"A1": 1, "A2": 2, "A4": 4, "B1": 11, "B2": 12, "B3": 13, "B7": 17}

	// Act
	events, err := cricsheet.BuildBallEventRows(
		context.Background(), identity, committedWicketKinds(t), match, 1, 9000010)
	rows := events.Deliveries

	// Assert
	require.NoError(t, err)
	require.Len(t, rows, 6, "the six deliveries of the two innings of the match, and no more")
	for i := range rows {
		assert.LessOrEqual(t, rows[i].Innings, 2, "delivery %d is in innings %d", i, rows[i].Innings)
		assertNotSuperOverPlayer(t, rows[i].StrikerID, identity["B7"], "B7 batted only in the super over")
		assertNotSuperOverPlayer(t, rows[i].BowlerID, identity["A4"], "A4 bowled only in the super over")
	}
}

// assertNotSuperOverPlayer fails when a resolved id on a stored ball is the player who
// appeared only in the super over.
func assertNotSuperOverPlayer(t *testing.T, got *int64, superOverOnly int64, reason string) {
	t.Helper()
	if got == nil {
		return
	}
	assert.NotEqual(t, superOverOnly, *got, reason)
}

// superOverSpyTx captures what the per-file transaction writes for the innings tables:
// match_inning upserts by innings number, and every innings number on a ball_event or
// batting_data row, whichever of the two write routes (row inserts, or a COPY into a
// staging table) the batch size selects.
type superOverSpyTx struct {
	matchInningNumbers []int
	ballEventInnings   []int
	battingInnings     []int
	bowlingInnings     []int
}

func (s *superOverSpyTx) Exec(_ context.Context, sql string, args ...any) error {
	switch {
	case strings.Contains(sql, "INSERT INTO match_inning") && len(args) >= matchInningArgCount:
		s.matchInningNumbers = append(s.matchInningNumbers, toInt(args[matchInningArgInningNumber]))
	case strings.Contains(sql, "INSERT INTO ball_event(") && len(args) > 1:
		s.ballEventInnings = append(s.ballEventInnings, toInt(args[1]))
	}
	return nil
}

func (s *superOverSpyTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}

func (s *superOverSpyTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row { return nopRow{} }

func (s *superOverSpyTx) CopyFrom(
	_ context.Context,
	table pgx.Identifier,
	_ []string,
	src pgx.CopyFromSource,
) (int64, error) {
	// Column 1 is the innings number on every one of these staging tables:
	// (match_id, innings, ...) for ball_event_stage, (match_id, inning_number, ...) for
	// batting_data_tmp and bowling_data_tmp.
	var into *[]int
	switch strings.Join(table, ".") {
	case "ball_event_stage":
		into = &s.ballEventInnings
	case "batting_data_tmp":
		into = &s.battingInnings
	case "bowling_data_tmp":
		into = &s.bowlingInnings
	}
	n := int64(0)
	for src.Next() {
		n++
		values, err := src.Values()
		if err != nil {
			return n, err
		}
		if into != nil && len(values) > 1 {
			*into = append(*into, toInt(values[1]))
		}
	}
	return n, src.Err()
}

func (s *superOverSpyTx) Commit(_ context.Context) error   { return nil }
func (s *superOverSpyTx) Rollback(_ context.Context) error { return nil }

func TestImportMatchFile_SuperOverFile_StoresOnlyTheInningsOfTheMatch(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	// Arrange
	ctx := context.Background()
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	spy := &superOverSpyTx{}
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spy)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
	file := writeTempJSON(t, t.TempDir(), "9000010.json", superOverTieJSON)

	// Act
	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, spy.matchInningNumbers, "exactly the two innings of the match")
	assert.Len(t, spy.ballEventInnings, 6, "the six deliveries of the match, none of the super over's")
	assertAllInningsAtMost(t, spy.ballEventInnings, 2, "ball_event")
	assert.Len(t, spy.battingInnings, 4, "A1, A2, B2 and B3 batted in the match; B7 only in the super over")
	assertAllInningsAtMost(t, spy.battingInnings, 2, "batting_data")
	assert.Len(t, spy.bowlingInnings, 2, "B1 and A1 bowled in the match; A4 only in the super over")
	assertAllInningsAtMost(t, spy.bowlingInnings, 2, "bowling_data")
}

// assertAllInningsAtMost fails when any captured innings number is above the limit.
func assertAllInningsAtMost(t *testing.T, innings []int, limit int, table string) {
	t.Helper()
	for i := range innings {
		assert.LessOrEqual(t, innings[i], limit, "%s row %d is in innings %d", table, i, innings[i])
	}
}
