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

// The acceptance test for IMPORT-05 on the real tables: a batter's balls and strike rate
// count the no-ball he faced and not the wide, while the bowler's balls, the innings'
// balls bowled and ball_event.is_legal count neither. The fixture is extrasMatchJSON,
// whose totals extras_test.go and faced_test.go explain.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

func TestImportMatchFile_NoBallAndWide_FacedIsNotLegal(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	// Arrange
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event_wicket, ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))
	const matchID = int64(9000011)
	path := filepath.Join(t.TempDir(), "9000011.json")
	require.NoError(t, os.WriteFile(path, []byte(extrasMatchJSON), 0o600))

	// Act
	require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))

	// Assert
	assert.Equal(t, battingFigure{runs: 5, balls: 4, strikeRate: 125}, battingFigureFor(ctx, t, matchID, "A1"),
		"A1 faced the four, the no-ball, the byes and the penalty ball, and not the wide")
	assert.Equal(t, 5, bowlingBallsFor(ctx, t, matchID, "B1"),
		"B1 is credited with neither the no-ball nor the wide")
	assert.Equal(t, 5, countForMatch(ctx, t,
		`SELECT balls_bowled FROM match_inning WHERE match_id = $1 AND inning_number = 1`, matchID),
		"the innings' balls are the bowler's count")
	assert.Equal(t, 5, countForMatch(ctx, t,
		`SELECT count(*) FROM ball_event WHERE match_id = $1 AND innings = 1 AND is_legal`, matchID),
		"is_legal is the bowler's count")
	assert.Equal(t, 6, countForMatch(ctx, t,
		`SELECT count(*) FROM ball_event WHERE match_id = $1 AND innings = 1 AND extras_wides = 0`, matchID),
		"a ball faced is a row without a wide, which is what the rating source reads")
}

// battingFigureFor reads the runs, balls and strike rate batting_data holds for the named
// batter in the match, resolved through the player table because the importer assigns
// the id.
func battingFigureFor(ctx context.Context, t *testing.T, matchID int64, playerName string) battingFigure {
	t.Helper()
	var figure battingFigure
	require.NoError(t, db.QueryRow(ctx, `
		SELECT b.runs, b.balls, b.strike_rate FROM batting_data b
		JOIN player p ON p.id = b.player_id
		WHERE b.match_id = $1 AND p.player_name = $2`, matchID, playerName).
		Scan(&figure.runs, &figure.balls, &figure.strikeRate))
	return figure
}

// bowlingBallsFor reads the balls bowling_data holds for the named bowler in the match.
func bowlingBallsFor(ctx context.Context, t *testing.T, matchID int64, playerName string) int {
	t.Helper()
	var balls int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT b.balls FROM bowling_data b
		JOIN player p ON p.id = b.player_id
		WHERE b.match_id = $1 AND p.player_name = $2`, matchID, playerName).Scan(&balls))
	return balls
}
