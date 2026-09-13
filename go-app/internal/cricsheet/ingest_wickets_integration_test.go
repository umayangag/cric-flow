package cricsheet_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// The acceptance test for IMPORT-06 on the real tables: a bowler's wickets are the kinds
// the scorecard credits him with, an innings' wickets lost leaves out the batter who
// retired hurt, and a delivery with two wickets keeps both. The fixture is
// wicketsMatchJSON, whose six wicket records wicket_test.go explains.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

func TestImportMatchFile_Wickets_CreditedLostAndAllRecorded(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	// Arrange
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event_wicket, ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))
	const matchID = int64(9000012)
	path := filepath.Join(t.TempDir(), "9000012.json")
	require.NoError(t, os.WriteFile(path, []byte(wicketsMatchJSON), 0o600))

	// Act
	require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))

	// Assert
	assert.Equal(t, 2, countForMatch(ctx, t, `
		SELECT b.wickets FROM bowling_data b JOIN player p ON p.id = b.player_id
		WHERE b.match_id = $1 AND p.player_name = 'B1'`, matchID),
		"B1 is credited with the catch and the bowled, not the run outs or the retirements")
	assert.Equal(t, 5, countForMatch(ctx, t,
		`SELECT wickets_lost FROM match_inning WHERE match_id = $1 AND inning_number = 1`, matchID),
		"five wickets lost: the retired-hurt batter is not out")
	assert.Equal(t, 6, countForMatch(ctx, t,
		`SELECT count(*) FROM ball_event_wicket WHERE match_id = $1`, matchID),
		"every wicket record in the file is a row")
	assert.Equal(t, 2, countForMatch(
		ctx,
		t,
		`SELECT count(*) FROM ball_event_wicket WHERE match_id = $1 AND innings = 1 AND "over" = 0 AND ball = 4`,
		matchID,
	),
		"the delivery with two wickets keeps both")
	assert.Equal(t, "run out", wicketKindFor(ctx, t, matchID, "A6"),
		"the second wicket on the delivery is the non-striker's run out, spelled as the vocabulary does")
	assert.Equal(t, "retired hurt", batterDescriptionFor(ctx, t, matchID, "A4"),
		"the scorecard still says the batter retired hurt")
}

// wicketKindFor reads the kind ball_event_wicket holds for the named batter's wicket.
func wicketKindFor(ctx context.Context, t *testing.T, matchID int64, playerName string) string {
	t.Helper()
	var kind string
	require.NoError(t, db.QueryRow(ctx, `
		SELECT w.kind FROM ball_event_wicket w JOIN player p ON p.id = w.player_out_id
		WHERE w.match_id = $1 AND p.player_name = $2`, matchID, playerName).Scan(&kind))
	return kind
}

// batterDescriptionFor reads the dismissal description batting_data holds for the batter.
func batterDescriptionFor(ctx context.Context, t *testing.T, matchID int64, playerName string) string {
	t.Helper()
	var description string
	require.NoError(t, db.QueryRow(ctx, `
		SELECT b.description FROM batting_data b JOIN player p ON p.id = b.player_id
		WHERE b.match_id = $1 AND p.player_name = $2`, matchID, playerName).Scan(&description))
	return description
}
