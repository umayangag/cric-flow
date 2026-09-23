package cricsheet_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// failOnSQLPool answers every statement the way nopPool does, except the one whose text
// contains `failOn`: that one fails. It is what a dimension row the database will not
// give back looks like to the importer.
type failOnSQLPool struct {
	nopPool
	failOn string
}

func (p failOnSQLPool) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	if strings.Contains(sql, p.failOn) {
		return failingRow{}
	}
	return p.nopPool.QueryRow(ctx, sql, args...)
}

type failingRow struct{}

func (failingRow) Scan(_ ...any) error { return errors.New("relation is unavailable") }

// matchFileNaming writes the smallest file the importer accepts, with whichever optional
// info fields the case is about. Names are unique per case because the entity cache is
// process-global: a team another test already resolved would be a cache hit and would
// never reach the pool.
func matchFileNaming(t *testing.T, teamA, teamB, season, tossWinner string) string {
	t.Helper()
	optional := ""
	if season != "" {
		optional += fmt.Sprintf(`"season": %q,`, season)
	}
	if tossWinner != "" {
		optional += fmt.Sprintf(`"toss": {"winner": %q, "decision": "bat"},`, tossWinner)
	}
	body := fmt.Sprintf(`{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-04-01"],
    "match_type": "T20",
    "team_type": "club",
    "gender": "male",
    %s
    "teams": [%q, %q]
  },
  "innings": [{"team":%q,"overs":[{"over":0,"deliveries":[
    {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":1,"extras":0,"total":1}}
  ]}]}]
}`, optional, teamA, teamB, teamA)
	dir := t.TempDir()
	path := filepath.Join(dir, "match.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// TestImportMatchFile_ADimensionLookupThatFails_FailsTheFile is IMPORT-13.
//
// Two lookups were written as `if id, e := lookup(); e == nil { use(id) }`: the season and
// the toss winner. A failure left the column NULL and said nothing, so the match was
// recorded as having been played in no season and with no toss -- both of which are things
// a file can legitimately not say, which is exactly why the silence was unreadable. The
// toss is a feature the win models read.
func TestImportMatchFile_ADimensionLookupThatFails_FailsTheFile(t *testing.T) {
	// Not parallel: the import path uses package-level singletons and RunInTxFn.
	testCases := []struct {
		name       string
		failOn     string
		teamA      string
		teamB      string
		season     string
		tossWinner string
		wantError  string
	}{
		{
			name:      "the season the file names",
			failOn:    "INSERT INTO season",
			teamA:     "Import13SeasonA",
			teamB:     "Import13SeasonB",
			season:    "import-13-season",
			wantError: `lookup season_id for "import-13-season"`,
		},
		{
			name:       "the side that won the toss",
			failOn:     "INSERT INTO opposition",
			teamA:      "Import13TossA",
			teamB:      "Import13TossB",
			tossWinner: "Import13TossA",
			wantError:  `get/create opposition for toss winner "Import13TossA"`,
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			// Arrange
			prevPool := db.PoolAPI
			db.SetPoolAPI(failOnSQLPool{failOn: testCase.failOn})
			t.Cleanup(func() { db.SetPoolAPI(prevPool) })
			cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
				return inner(ctx, ingestErrorTestNopTx{})
			})
			t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
			file := matchFileNaming(t, testCase.teamA, testCase.teamB, testCase.season, testCase.tossWinner)

			// Act
			err := cricsheet.ImportMatchFile(context.Background(), file, &cricsheet.Options{})

			// Assert
			require.Error(t, err, "a lookup that failed is not a column the file left blank")
			assert.ErrorContains(t, err, testCase.wantError)
		})
	}
}

// idAssigningPool hands back a real id for every get-or-create, so a unit test can see
// what the import would have written rather than the zero a nop pool leaves behind.
type idAssigningPool struct {
	nopPool
}

func (p idAssigningPool) QueryRow(_ context.Context, _ string, args ...any) db.Row {
	return assignedIDRow{args: args}
}

type assignedIDRow struct {
	args []any
}

// Scan fills an id and, where the statement asks for it, the stored name. The id is
// derived from the first argument so two calls for one person agree.
func (r assignedIDRow) Scan(dest ...any) error {
	name := ""
	if len(r.args) > 0 {
		name = fmt.Sprintf("%v", r.args[0])
	}
	for _, d := range dest {
		switch target := d.(type) {
		case *int64:
			*target = int64(len(name)) + 1
		case *string:
			*target = name
		}
	}
	return nil
}

// settlementNamesFile is one file with names no other case in this package uses. The
// entity cache is process-global: a name another test already resolved against a nop pool
// is a cache hit holding id 0, and a player id of 0 never reaches settlement at all.
const settlementNamesFile = `{"info":{"match_type":"T20","team_type":"club",` +
	`"teams":["Import13NameA","Import13NameB"],"dates":["2024-05-01"],"gender":"male"},` +
	`"innings":[{"team":"Import13NameA","overs":[{"over":0,"deliveries":[{"batter":"Import13Striker",` +
	`"bowler":"Import13Bowler","non_striker":"Import13NonStriker","runs":{"batter":1,"extras":0,"total":1}}]}]}]}`

// TestImportDir_SpellingsOfAFileWhoseTransactionFailed_DoNotSettleDisplayNames is
// IMPORT-13.
//
// A player's display name is settled across a whole import from the spellings the files
// used. The spellings were collected as each name was resolved -- before the file's rows
// were written -- so a file whose transaction rolled back still voted, and could rename a
// player on the strength of a match the archive does not hold. They are handed over after
// the commit now, so only a file that landed has a say.
func TestImportDir_SpellingsOfAFileWhoseTransactionFailed_DoNotSettleDisplayNames(t *testing.T) {
	// Not parallel: the import path uses package-level singletons and RunInTxFn.
	testCases := []struct {
		name          string
		transactionOK bool
		wantAnyNames  bool
	}{
		{name: "the file commits, so its spellings count", transactionOK: true, wantAnyNames: true},
		{name: "the file rolls back, so they do not", transactionOK: false, wantAnyNames: false},
	}
	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			// Arrange
			harness := newSettlementHarness(t, map[string]string{"match1.json": settlementNamesFile})
			previousPool := db.PoolAPI
			db.SetPoolAPI(idAssigningPool{})
			t.Cleanup(func() { db.SetPoolAPI(previousPool) })
			transactionError := errors.New("db error")
			cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
				if err := inner(ctx, ingestErrorTestNopTx{}); err != nil {
					return err
				}
				if !testCase.transactionOK {
					return transactionError
				}
				return nil
			})
			t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })

			var settled []db.PlayerDisplayName
			harness.database.EXPECT().
				UpdatePlayerDisplayNames(mock.Anything, mock.Anything).
				Run(func(_ context.Context, names []db.PlayerDisplayName) { settled = names }).
				Return(nil).
				Once()
			harness.database.EXPECT().
				ApplyTeamLineage(mock.Anything, mock.Anything).
				Return(db.TeamLineageReport{}, nil).
				Once()

			// Act
			_, _ = cricsheet.ImportDir(context.Background(), harness.dir, &cricsheet.Options{}, 1)

			// Assert
			assert.Equal(t, testCase.wantAnyNames, len(settled) > 0,
				"only a file whose rows reached the archive may settle a display name")
			harness.database.AssertExpectations(t)
		})
	}
}
