package cricsheet_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// The acceptance test for IMPORT-01 on the real tables: a file with a super over produces
// exactly two match_inning rows, no ball_event with innings above 2, and no scorecard row
// for a player who appeared only in the super over.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

// superOverOf renders one super-over innings of a single over: the same shape inningsOf
// renders, with Cricsheet's super_over flag on it.
func superOverOf(team, batter, nonStriker, bowler string, runs []int) string {
	deliveries := make([]string, 0, len(runs))
	for i := range runs {
		deliveries = append(deliveries, deliveryOf(batter, nonStriker, bowler, runs[i], ""))
	}
	return fmt.Sprintf(`{"team":"%s","super_over":true,"overs":[{"over":0,"deliveries":[%s]}]}`,
		team, strings.Join(deliveries, ","))
}

func TestImportMatchFile_SuperOverFile_LeavesTheSuperOverOutOfEveryTable(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	// Arrange
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))
	const matchID = int64(9000010)
	path := filepath.Join(t.TempDir(), fmt.Sprintf("%d.json", matchID))
	// B7 bats and A4 bowls only in the super over.
	file := matchFileJSON(
		inningsOf("Alpha", "A1", "A2", "B1", []int{4, 1, 6}, ""),
		inningsOf("Beta", "B2", "B3", "A1", []int{4, 6, 1}, "A3"),
		superOverOf("Beta", "B7", "B2", "A4", []int{6, 0}),
		superOverOf("Alpha", "A1", "A2", "B1", []int{1, 0}),
	)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o600))

	// Act
	require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))

	// Assert
	require.Equal(t, 2, countForMatch(ctx, t,
		`SELECT count(*) FROM match_inning WHERE match_id = $1`, matchID),
		"exactly the two innings of the match")
	require.Equal(t, 0, countForMatch(ctx, t,
		`SELECT count(*) FROM ball_event WHERE match_id = $1 AND innings > 2`, matchID),
		"no delivery of the super over")
	require.Equal(t, 6, countForMatch(ctx, t,
		`SELECT count(*) FROM ball_event WHERE match_id = $1`, matchID),
		"the six deliveries of the match")
	require.Equal(t, 0, countBattingRowsFor(ctx, t, matchID, "B7"),
		"B7 batted only in the super over and has no innings of the match")
	require.Equal(t, 0, countBowlingRowsFor(ctx, t, matchID, "A4"),
		"A4 bowled only in the super over and has no innings of the match")
	require.Equal(t, 1, countBowlingRowsFor(ctx, t, matchID, "B1"),
		"B1's one innings of the match, not the super over as a second")
}

// countBattingRowsFor returns how many batting_data rows the match holds for a player
// named playerName, resolved through the player table because the importer assigns the id.
func countBattingRowsFor(ctx context.Context, t *testing.T, matchID int64, playerName string) int {
	t.Helper()
	return countForMatch(ctx, t, `
		SELECT count(*) FROM batting_data b
		JOIN player p ON p.id = b.player_id
		WHERE b.match_id = $1 AND p.player_name = $2`, matchID, playerName)
}
