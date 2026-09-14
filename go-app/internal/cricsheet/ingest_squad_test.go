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

// squadJSON is sampleJSON's fixture with info.players, and deliberately lists players
// who never bat or bowl (A4, B4). Those are exactly the ones the scorecard loses and
// the reason match_player exists.
const squadJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-01-02"],
    "match_type": "T20",
    "teams": ["Alpha", "Beta"],
    "venue": "The Oval",
    "season": "2024",
    "players": {
      "Alpha": ["A1", "A2", "A3", "A4"],
      "Beta": ["B1", "B2", "B3", "B4"]
    },
    "outcome": {"winner": "Alpha"}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":1,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}}
      ]}
    ]},
    {"team":"Beta","overs":[
      {"over":1,"deliveries":[
        {"batter":"B1","bowler":"A1","non_striker":"B2","runs":{"batter":1,"extras":0,"total":1}}
      ]}
    ]}
  ]
}`

// noSquadJSON is the same match with info.players absent.
const noSquadJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-01-02"],
    "match_type": "T20",
    "teams": ["Alpha", "Beta"],
    "season": "2024"
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":1,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}}
      ]}
    ]},
    {"team":"Beta","overs":[
      {"over":1,"deliveries":[
        {"batter":"B1","bowler":"A1","non_striker":"B2","runs":{"batter":1,"extras":0,"total":1}}
      ]}
    ]}
  ]
}`

// namesakeSquadJSON lists the same player name for both sides: two people sharing a
// scorecard name, which Cricsheet's registry collapses into one identifier.
const namesakeSquadJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-01-02"],
    "match_type": "T20",
    "teams": ["Alpha", "Beta"],
    "season": "2024",
    "players": {"Alpha": ["A1", "A2"], "Beta": ["A1", "B2"]}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":1,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}}
      ]}
    ]}
  ]
}`

// matchPlayerInsertArgs is how many values one match_player row is inserted with:
// (match_id, player_id, opposition_id, is_replacement).
const matchPlayerInsertArgs = 4

// squadSpyTx records the match_player statements the importer runs.
type squadSpyTx struct {
	deletes     int
	insertArgs  []any
	insertCount int
}

func (t *squadSpyTx) Exec(_ context.Context, sql string, args ...any) error {
	switch {
	case strings.Contains(sql, "DELETE FROM match_player"):
		t.deletes++
	case strings.Contains(sql, "INSERT INTO match_player"):
		t.insertCount++
		t.insertArgs = append(t.insertArgs, args...)
	}
	return nil
}

func (t *squadSpyTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}
func (t *squadSpyTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row { return nopRow{} }

func (t *squadSpyTx) CopyFrom(
	_ context.Context,
	_ pgx.Identifier,
	_ []string,
	src pgx.CopyFromSource,
) (int64, error) {
	n := int64(0)
	for src.Next() {
		n++
	}
	return n, src.Err()
}

func (t *squadSpyTx) Commit(_ context.Context) error   { return nil }
func (t *squadSpyTx) Rollback(_ context.Context) error { return nil }

// importWithSquadSpy runs one file through the importer against a spy transaction.
func importWithSquadSpy(t *testing.T, contents string) (*squadSpyTx, error) {
	t.Helper()
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })

	spy := &squadSpyTx{}
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spy)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })

	file := writeTempJSON(t, t.TempDir(), "m.json", contents)
	return spy, cricsheet.ImportMatchFile(context.Background(), file, &cricsheet.Options{})
}

func TestImportMatchFile_WithPlayers_WritesTheWholeSquad(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	// Arrange + Act
	spy, err := importWithSquadSpy(t, squadJSON)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, spy.deletes, "the previous squad is cleared exactly once")
	assert.Equal(t, 1, spy.insertCount, "one multi-row insert, not one per player")
	// Eight players across both sides, four arguments each. A4 and B4 never appear in
	// the scorecard, so a count of 8 is what distinguishes this from the old behaviour.
	assert.Len(t, spy.insertArgs, 8*matchPlayerInsertArgs)
}

func TestImportMatchFile_WithoutPlayers_ImportsTheMatchAndRecordsNoSquad(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	// Arrange + Act
	spy, err := importWithSquadSpy(t, noSquadJSON)

	// Assert
	require.NoError(t, err, "a missing squad must not cost us the ball-by-ball record")
	assert.Equal(t, 1, spy.deletes, "any stale squad is still cleared")
	assert.Zero(t, spy.insertCount, "no squad means no rows, not a side of nobody")
}

func TestImportMatchFile_PlayerOnBothTeams_ImportsWithoutThatPlayer(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	// Arrange + Act
	spy, err := importWithSquadSpy(t, namesakeSquadJSON)

	// Assert
	require.NoError(t, err, "an unresolvable name must not cost the whole match")
	assert.Equal(t, 1, spy.insertCount)
	// A1 is named by both sides and dropped from both, leaving A2 and B2.
	assert.Len(t, spy.insertArgs, 2*matchPlayerInsertArgs)
}

// replacementSquadJSON is squadJSON with a fifth Alpha player, A5, who came in for A3 on
// the second ball as a concussion substitute (FEAT-02). Alpha lists five and started four.
const replacementSquadJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-01-02"],
    "match_type": "T20",
    "teams": ["Alpha", "Beta"],
    "venue": "The Oval",
    "season": "2024",
    "players": {
      "Alpha": ["A1", "A2", "A3", "A4", "A5"],
      "Beta": ["B1", "B2", "B3", "B4"]
    },
    "outcome": {"winner": "Alpha"}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":1,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0},
         "replacements":{"match":[{"in":"A5","out":"A3","reason":"concussion_substitute","team":"Alpha"}]}}
      ]}
    ]},
    {"team":"Beta","overs":[
      {"over":1,"deliveries":[
        {"batter":"B1","bowler":"A5","non_striker":"B2","runs":{"batter":1,"extras":0,"total":1}}
      ]}
    ]}
  ]
}`

func TestImportMatchFile_WithAReplacement_FlagsTheManWhoCameInAndNobodyElse(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	// Arrange + Act
	spy, err := importWithSquadSpy(t, replacementSquadJSON)

	// Assert
	require.NoError(t, err)
	require.Len(t, spy.insertArgs, 9*matchPlayerInsertArgs, "everyone listed is a row, the replacement included")
	// Rows follow info.players order, four arguments each, the flag last: A5 is the fifth
	// row and the only one flagged.
	flags := make([]any, 0, 9)
	for i := matchPlayerInsertArgs - 1; i < len(spy.insertArgs); i += matchPlayerInsertArgs {
		flags = append(flags, spy.insertArgs[i])
	}
	assert.Equal(t, []any{false, false, false, false, true, false, false, false, false}, flags)
}
