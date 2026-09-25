package cricsheet_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// This suite is the measurement behind IMPORT-13, written as a test so the gap is a fact
// in the build rather than a paragraph in an audit.
//
// A Cricsheet file's dimension rows -- its ground, its season, its clubs, its players --
// are created by EntityCache on the pool, in their own autocommitted statements, before
// the per-file transaction opens. The transaction covers the facts (match, innings,
// deliveries, scorecards) and nothing else. So a file that fails anywhere after its first
// dimension row rolls back every fact it wrote and leaves every dimension row it created
// standing, referenced by nothing.
//
// The assertions below are deliberately written the way the importer behaves TODAY: they
// assert the leak. They exist so that the day someone closes IMPORT-13 -- by resolving
// dimensions through the file's own transaction -- the four `wantRowsToday: 1` values
// become 0 and this suite becomes the acceptance test for that change, naming the exact
// rows that must not survive a failed file. Until then it pins the blast radius: what
// leaks, and what does not.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

// leakFixtureDate is the date the fixture match is played on. Nothing here turns on it.
const leakFixtureDate = "2024-03-04"

// The fixture's names are prefixed so they cannot collide with any row another suite in
// this package leaves behind, and so the cleanup below can delete exactly its own rows.
const (
	leakVenueName   = "IMPORT13 Leak Ground"
	leakSeasonName  = "IMPORT13 Leak Season"
	leakBattingTeam = "IMPORT13 Leak Alpha"
	leakBowlingTeam = "IMPORT13 Leak Beta"
	leakStrikerName = "IMPORT13 Leak Striker"
	leakNonStriker  = "IMPORT13 Leak Non-Striker"
	leakBowlerName  = "IMPORT13 Leak Bowler"
	leakMatchFile   = "9100013.json"
)

// failingMatchFileJSON renders a file whose first innings is complete and whose second
// names no team.
//
// That second innings is what fails the file, and where it fails is the point: the check
// that rejects it (`inning %d has no team name`) sits after the ground, the season, both
// clubs and the first innings' players have all been created, and before a single fact
// row has been committed. It is the ordinary shape of a mid-file failure, reproduced
// deterministically from the file's own contents rather than by breaking the database.
func failingMatchFileJSON() string {
	return fmt.Sprintf(`{
  "info": {
    "balls_per_over": 6,
    "dates": ["%s"],
    "match_type": "T20",
    "team_type": "club",
    "teams": ["%s", "%s"],
    "venue": "%s",
    "city": "Leaktown",
    "season": "%s",
    "gender": "male",
    "toss": {"winner": "%s"},
    "outcome": {"winner": "%s"},
    "registry": {"people": {
      "%s": "import13-striker",
      "%s": "import13-non-striker",
      "%s": "import13-bowler"
    }}
  },
  "innings": [
    {"team": "%s", "overs": [{"over": 0, "deliveries": [
      {"batter": "%s", "bowler": "%s", "non_striker": "%s",
       "runs": {"batter": 4, "extras": 0, "total": 4}}
    ]}]},
    {"team": "", "overs": []}
  ]
}`,
		leakFixtureDate,
		leakBattingTeam, leakBowlingTeam,
		leakVenueName,
		leakSeasonName,
		leakBattingTeam,
		leakBattingTeam,
		leakStrikerName, leakNonStriker, leakBowlerName,
		leakBattingTeam,
		leakStrikerName, leakBowlerName, leakNonStriker,
	)
}

// deleteLeakFixtureRows removes the fixture's dimension rows. The suite runs it before and
// after the import: before, so a previous run cannot make the assertions pass by accident;
// after, so the leak this suite provokes does not outlive it. Facts are deleted first
// because the dimension rows are what they point at.
func deleteLeakFixtureRows(ctx context.Context, t *testing.T) {
	t.Helper()
	require.NoError(t, db.Exec(ctx,
		`DELETE FROM match WHERE venue_id IN (SELECT id FROM venue WHERE venue_name = $1)`,
		leakVenueName))
	require.NoError(t, db.Exec(ctx, `DELETE FROM venue WHERE venue_name = $1`, leakVenueName))
	require.NoError(t, db.Exec(ctx, `DELETE FROM season WHERE season_name = $1`, leakSeasonName))
	require.NoError(t, db.Exec(ctx,
		`DELETE FROM opposition WHERE opposition_name = ANY($1)`,
		[]string{leakBattingTeam, leakBowlingTeam}))
	require.NoError(t, db.Exec(ctx,
		`DELETE FROM player WHERE player_name = ANY($1)`,
		[]string{leakStrikerName, leakNonStriker, leakBowlerName}))
}

// countRows answers one counting query. The query comes from the caller because each
// dimension table names its natural key differently.
func countRows(ctx context.Context, t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(ctx, query, args...).Scan(&count))
	return count
}

func TestImportMatchFile_FileFailsMidway_DimensionRowsSurviveTheRollback(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	deleteLeakFixtureRows(ctx, t)
	t.Cleanup(func() { deleteLeakFixtureRows(context.Background(), t) })

	dir := t.TempDir()
	path := filepath.Join(dir, leakMatchFile)
	require.NoError(t, os.WriteFile(path, []byte(failingMatchFileJSON()), 0o600))

	importErr := cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{})
	require.Error(t, importErr, "the second innings names no team, so the file must fail")
	require.Contains(t, importErr.Error(), "inning 2 has no team name",
		"it must fail at the innings check, not somewhere earlier or later")

	testCases := []struct {
		name string
		// query counts the rows the failed file is responsible for.
		query string
		args  []any
		// wantRowsToday is what the importer leaves behind as it stands. Every 1 here is
		// a row IMPORT-13 says should not exist; closing it turns them into 0.
		wantRowsToday int
		reason        string
	}{
		{
			name:          "the ground the file named survives",
			query:         `SELECT count(*) FROM venue WHERE venue_name = $1`,
			args:          []any{leakVenueName},
			wantRowsToday: 1,
			reason:        "and is offered by /api/options/venues, with no match behind it",
		},
		{
			name:          "the season the file named survives",
			query:         `SELECT count(*) FROM season WHERE season_name = $1`,
			args:          []any{leakSeasonName},
			wantRowsToday: 1,
			reason:        "created for a match that was never recorded",
		},
		{
			name:          "both clubs the file named survive",
			query:         `SELECT count(*) FROM opposition WHERE opposition_name = ANY($1)`,
			args:          []any{[]string{leakBattingTeam, leakBowlingTeam}},
			wantRowsToday: 2,
			reason:        "the winner and toss lookups created them before the innings check ran",
		},
		{
			name:          "the first innings' players survive",
			query:         `SELECT count(*) FROM player WHERE player_name = ANY($1)`,
			args:          []any{[]string{leakStrikerName, leakNonStriker, leakBowlerName}},
			wantRowsToday: 3,
			reason:        "the innings that parsed resolved them before the innings that did not",
		},
		{
			name:          "no match row survives",
			query:         `SELECT count(*) FROM match WHERE venue_id IN (SELECT id FROM venue WHERE venue_name = $1)`,
			args:          []any{leakVenueName},
			wantRowsToday: 0,
			reason:        "the facts are inside the transaction, and the transaction rolled back",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			got := countRows(ctx, t, testCase.query, testCase.args...)
			require.Equal(t, testCase.wantRowsToday, got, testCase.reason)
		})
	}
}
