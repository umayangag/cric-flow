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

// The acceptance test for IMPORT-04 on the real tables: a no-ball with four leg-byes is
// stored with both kinds on its ball_event row, and the bowler's bowling_data row is
// charged the no-ball and not the leg-byes. The fixture is extrasMatchJSON, whose totals
// extras_test.go explains.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

// storedExtras is one ball_event row's extras as the table holds them.
type storedExtras struct {
	wides, noBalls, byes, legByes, penalty int
	runsExtras                             int
	kind                                   *string
}

func TestImportMatchFile_NoBallWithFourLegByes_IsRecoverableAndNotTheBowlers(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	// Arrange
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))
	const matchID = int64(9000011)
	path := filepath.Join(t.TempDir(), "9000011.json")
	require.NoError(t, os.WriteFile(path, []byte(extrasMatchJSON), 0o600))

	// Act
	require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))

	// Assert
	var got storedExtras
	require.NoError(t, db.QueryRow(ctx, `
		SELECT extras_wides, extras_noballs, extras_byes, extras_legbyes, extras_penalty,
		       runs_extras, extras_kind
		FROM ball_event
		WHERE match_id = $1 AND innings = 1 AND "over" = 0 AND ball = 2`, matchID).
		Scan(&got.wides, &got.noBalls, &got.byes, &got.legByes, &got.penalty,
			&got.runsExtras, &got.kind))
	assert.Equal(t, storedExtras{noBalls: 1, legByes: 4, runsExtras: 5, kind: stringPointer("no_ball")}, got,
		"the summary still reads no_ball alone; the breakdown says four of the five were leg-byes")

	runs, maidens := bowlingFigureFor(ctx, t, matchID, "B1")
	assert.Equal(t, bowlingFigure{runs: 7, maidens: 0}, bowlingFigure{runs: runs, maidens: maidens},
		"B1 concedes the bat's runs, the no-ball and the wide, not the leg-byes, byes or penalty")
	runs, maidens = bowlingFigureFor(ctx, t, matchID, "A1")
	assert.Equal(t, bowlingFigure{runs: 0, maidens: 1}, bowlingFigure{runs: runs, maidens: maidens},
		"four byes are none of A1's, and his over is a maiden")
	assert.Equal(t, 20, countForMatch(ctx, t,
		`SELECT runs_scored FROM match_inning WHERE match_id = $1 AND inning_number = 1`, matchID),
		"the innings keeps every run")
}

// bowlingFigureFor reads the runs and maidens bowling_data holds for the named bowler in
// the match, resolved through the player table because the importer assigns the id.
func bowlingFigureFor(ctx context.Context, t *testing.T, matchID int64, playerName string) (int, int) {
	t.Helper()
	var runs, maidens int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT b.runs, b.maidens FROM bowling_data b
		JOIN player p ON p.id = b.player_id
		WHERE b.match_id = $1 AND p.player_name = $2`, matchID, playerName).Scan(&runs, &maidens))
	return runs, maidens
}
