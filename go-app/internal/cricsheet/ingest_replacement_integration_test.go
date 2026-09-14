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

// The acceptance test for FEAT-02 on the real table: a side that used a concussion
// substitute is stored in full, and the row of the man who came in -- and only his --
// carries is_replacement (migration 0020), so a reader that wants the eleven that started
// can leave him out. The fixture is replacementSquadJSON: Alpha lists five and A5 came in
// for A3 on the second ball.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

func TestImportMatchFile_Replacement_StoredAndFlaggedOnHisRowOnly(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	// Arrange
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event_wicket, ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))
	const matchID = int64(9000020)
	path := filepath.Join(t.TempDir(), "9000020.json")
	require.NoError(t, os.WriteFile(path, []byte(replacementSquadJSON), 0o600))

	// Act
	require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))

	// Assert
	assert.Equal(t, 9, countForMatch(ctx, t,
		`SELECT count(*) FROM match_player WHERE match_id = $1`, matchID),
		"everyone the file lists is a row, the replacement included")
	assert.Equal(t, 1, countForMatch(ctx, t,
		`SELECT count(*) FROM match_player WHERE match_id = $1 AND is_replacement`, matchID),
		"one man came in")
	assert.Equal(t, "A5", replacementNameFor(ctx, t, matchID),
		"and it is the `in` of the replacements.match entry")
	assert.Equal(t, 4, countForMatch(ctx, t, `
		SELECT count(*) FROM match_player mp JOIN opposition o ON o.id = mp.opposition_id
		WHERE mp.match_id = $1 AND o.opposition_name = 'Alpha' AND NOT mp.is_replacement`, matchID),
		"the side that started is the list without him")
}

// replacementNameFor reads the name of the one flagged member of the match.
func replacementNameFor(ctx context.Context, t *testing.T, matchID int64) string {
	t.Helper()
	var name string
	require.NoError(t, db.QueryRow(ctx, `
		SELECT p.player_name FROM match_player mp JOIN player p ON p.id = mp.player_id
		WHERE mp.match_id = $1 AND mp.is_replacement`, matchID).Scan(&name))
	return name
}
