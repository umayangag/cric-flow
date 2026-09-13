package cricsheet_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// This suite is the acceptance test for IMPORT-03: a corrected Cricsheet file republished
// under the same match id must leave the database describing the new file and nothing of
// the old one. Every case imports a match, overwrites the file in place -- which is what
// Cricsheet does, and what keeps the match id the same -- and imports it again.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

// reimportFixtureDate is the match date every fixture below is played on. It is only ever
// one date because nothing in these cases turns on it.
const reimportFixtureDate = "2024-01-02"

// matchFileJSON renders a Cricsheet file whose innings are the ones given.
func matchFileJSON(innings ...string) string {
	return fmt.Sprintf(`{
  "info": {
    "balls_per_over": 6,
    "dates": ["%s"],
    "match_type": "T20",
    "teams": ["Alpha", "Beta"],
    "venue": "The Oval",
    "season": "2024",
    "gender": "male",
    "toss": {"winner": "Alpha"},
    "outcome": {"winner": "Alpha"}
  },
  "innings": [%s]
}`, reimportFixtureDate, strings.Join(innings, ","))
}

// inningsOf renders one innings of a single over, one delivery per entry in runs, all
// faced by batter off bowler. A caughtBy that is not empty makes the last delivery a
// catch, so the import writes fielding_event and fielding_data rows for that innings.
func inningsOf(team, batter, nonStriker, bowler string, runs []int, caughtBy string) string {
	deliveries := make([]string, 0, len(runs))
	for i := range runs {
		fielder := ""
		if i == len(runs)-1 {
			fielder = caughtBy
		}
		deliveries = append(deliveries, deliveryOf(batter, nonStriker, bowler, runs[i], fielder))
	}
	return fmt.Sprintf(`{"team":"%s","overs":[{"over":0,"deliveries":[%s]}]}`,
		team, strings.Join(deliveries, ","))
}

// deliveryOf renders one delivery, with a catch by caughtBy when that is not empty.
func deliveryOf(batter, nonStriker, bowler string, runs int, caughtBy string) string {
	wicket := ""
	if caughtBy != "" {
		wicket = fmt.Sprintf(
			`,"wickets":[{"player_out":"%s","kind":"caught","fielders":[{"name":"%s"}]}]`,
			batter, caughtBy,
		)
	}
	return fmt.Sprintf(
		`{"batter":"%s","bowler":"%s","non_striker":"%s","runs":{"batter":%d,"extras":0,"total":%d}%s}`,
		batter, bowler, nonStriker, runs, runs, wicket,
	)
}

// countForMatch runs a counting query and returns the count. The query comes from the
// caller because the fact tables disagree on what they call an innings.
func countForMatch(ctx context.Context, t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(ctx, query, args...).Scan(&count))
	return count
}

// importFileTwice writes contents to the same path twice, importing after each write, and
// returns the match id both imports resolved to. Writing to one path is what makes this a
// re-import rather than two matches: the match id comes from the file name.
func importFileTwice(ctx context.Context, t *testing.T, dir, fileName, first, second string) int64 {
	t.Helper()
	path := filepath.Join(dir, fileName)
	require.NoError(t, os.WriteFile(path, []byte(first), 0o600))
	require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))
	require.NoError(t, os.WriteFile(path, []byte(second), 0o600))
	require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))
	matchID, err := strconv.ParseInt(strings.TrimSuffix(fileName, ".json"), 10, 64)
	require.NoError(t, err)
	return matchID
}

func TestImportMatchFile_ReImportOfACorrectedFile_LeavesNothingOfTheOldOne(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	// Unwired, not merely closed: the offline tests in this package are written against
	// db.Pool being nil, which is what makes the entity cache resolve no ids. Closing the
	// pool while the package-level handle still points at it left them talking to a closed
	// connection instead.
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))

	testCases := []struct {
		name     string
		fileName string
		first    string
		second   string
		assert   func(t *testing.T, matchID int64)
	}{
		{
			name:     "an innings the corrected file no longer has leaves no rows behind",
			fileName: "9000001.json",
			first: matchFileJSON(
				inningsOf("Alpha", "A1", "A2", "B1", []int{4, 1, 6}, ""),
				inningsOf("Beta", "B2", "B3", "A1", []int{1, 0}, "A3"),
			),
			second: matchFileJSON(inningsOf("Alpha", "A1", "A2", "B1", []int{4, 1, 6}, "")),
			assert: func(t *testing.T, matchID int64) {
				assertNoRowsForInnings(t, matchID, 2)
				require.Equal(t, 3, countForMatch(ctx, t,
					`SELECT count(*) FROM ball_event WHERE match_id = $1`, matchID),
					"only the surviving innings' deliveries")
				require.Equal(t, 0, countForMatch(ctx, t,
					`SELECT count(*) FROM fielding_event WHERE match_id = $1`, matchID),
					"the catch was in the innings that vanished")
				require.Equal(t, 0, countForMatch(ctx, t,
					`SELECT count(*) FROM fielding_data WHERE match_id = $1`, matchID),
					"and so is the fielding aggregate derived from it")
			},
		},
		{
			name:     "a corrected delivery replaces the one imported before it",
			fileName: "9000002.json",
			first:    matchFileJSON(inningsOf("Alpha", "A1", "A2", "B1", []int{4, 1, 6}, "")),
			second:   matchFileJSON(inningsOf("Alpha", "A1", "A2", "B1", []int{0, 1, 6}, "")),
			assert: func(t *testing.T, matchID int64) {
				var runsOnFirstBall int
				require.NoError(t, db.QueryRow(ctx,
					`SELECT runs_total FROM ball_event
					 WHERE match_id = $1 AND innings = 1 AND "over" = 0 AND ball = 1`,
					matchID).Scan(&runsOnFirstBall))
				require.Equal(t, 0, runsOnFirstBall, "the correction, not the first import")
				var runsScored int
				require.NoError(t, db.QueryRow(ctx,
					`SELECT runs_scored FROM match_inning WHERE match_id = $1 AND inning_number = 1`,
					matchID).Scan(&runsScored))
				require.Equal(t, 7, runsScored)
			},
		},
		{
			name:     "a bowler the corrected file no longer names keeps no bowling row",
			fileName: "9000003.json",
			first:    matchFileJSON(inningsOf("Alpha", "A1", "A2", "B1", []int{4, 1, 6}, "")),
			second:   matchFileJSON(inningsOf("Alpha", "A1", "A2", "B9", []int{4, 1, 6}, "")),
			assert: func(t *testing.T, matchID int64) {
				require.Equal(t, 0, countBowlingRowsFor(ctx, t, matchID, "B1"),
					"the bowler of the first import is not in the corrected file")
				require.Equal(t, 1, countBowlingRowsFor(ctx, t, matchID, "B9"))
			},
		},
	}

	dir := t.TempDir()
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange and act: import, correct the file in place, import again.
			matchID := importFileTwice(ctx, t, dir, tc.fileName, tc.first, tc.second)

			// Assert
			tc.assert(t, matchID)
		})
	}
}

// assertNoRowsForInnings fails when any fact table still holds the given innings of the
// match. Every table names the column differently, which is the whole of this helper.
func assertNoRowsForInnings(t *testing.T, matchID int64, innings int) {
	t.Helper()
	ctx := context.Background()
	queries := map[string]string{
		"ball_event":     `SELECT count(*) FROM ball_event WHERE match_id = $1 AND innings = $2`,
		"match_inning":   `SELECT count(*) FROM match_inning WHERE match_id = $1 AND inning_number = $2`,
		"batting_data":   `SELECT count(*) FROM batting_data WHERE match_id = $1 AND inning_number = $2`,
		"bowling_data":   `SELECT count(*) FROM bowling_data WHERE match_id = $1 AND inning_number = $2`,
		"fielding_data":  `SELECT count(*) FROM fielding_data WHERE match_id = $1 AND inning_number = $2`,
		"fielding_event": `SELECT count(*) FROM fielding_event WHERE match_id = $1 AND innings = $2`,
	}
	for table, query := range queries {
		require.Equal(t, 0, countForMatch(ctx, t, query, matchID, innings),
			"%s still holds innings %d of match %d", table, innings, matchID)
	}
}

// countBowlingRowsFor returns how many bowling_data rows the match holds for a player
// named playerName. The name is resolved through the player table because the importer
// assigns the id.
func countBowlingRowsFor(ctx context.Context, t *testing.T, matchID int64, playerName string) int {
	t.Helper()
	return countForMatch(ctx, t, `
		SELECT count(*) FROM bowling_data b
		JOIN player p ON p.id = b.player_id
		WHERE b.match_id = $1 AND p.player_name = $2`, matchID, playerName)
}
