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

// The acceptance test for IMPORT-11 on the real tables: a rain-revised chase reaches
// match_inning with the runs and the over limit the file states, and a first-class second
// innings reaches it with neither. The unit test beside this one (ingest_target_test.go)
// pins the same rule at the upsert's argument list; this one proves the two columns exist
// and round-trip through Postgres.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

func TestImportMatchFile_RevisedTargetReachesMatchInning(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	// Arrange
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event_wicket, ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))
	const revisedMatchID = int64(9000110)
	const firstClassMatchID = int64(9000111)
	dir := t.TempDir()
	revised := filepath.Join(dir, "9000110.json")
	require.NoError(t, os.WriteFile(
		revised,
		[]byte(matchFileForTarget("ODI", `{"runs":235,"overs":42.3}`)),
		0o600,
	))
	firstClass := filepath.Join(dir, "9000111.json")
	require.NoError(t, os.WriteFile(firstClass, []byte(matchFileForTarget("MDM", "")), 0o600))

	// Act
	require.NoError(t, cricsheet.ImportMatchFile(ctx, revised, &cricsheet.Options{}))
	require.NoError(t, cricsheet.ImportMatchFile(ctx, firstClass, &cricsheet.Options{}))

	// Assert
	assert.Equal(t, "235 runs in 42.3 overs", storedTargetFor(ctx, t, revisedMatchID, 2),
		"the D/L figure the archive states, not the 12 the first innings implies")
	assert.Equal(t, "no target", storedTargetFor(ctx, t, revisedMatchID, 1),
		"nobody is chasing during the first innings")
	assert.Equal(t, "no target", storedTargetFor(ctx, t, firstClassMatchID, 2),
		"a first-class second innings is not a chase")
}

// storedTargetFor reads match_inning's target back as the same one-line description the
// unit test asserts on, so a failure names the runs and the over limit rather than two
// nullable scan targets.
func storedTargetFor(ctx context.Context, t *testing.T, matchID int64, inningNumber int) string {
	t.Helper()
	var mi db.MatchInningInsert
	require.NoError(t, db.QueryRow(ctx, `
		SELECT target_runs, target_overs FROM match_inning
		WHERE match_id = $1 AND inning_number = $2`, matchID, inningNumber).
		Scan(&mi.TargetRuns, &mi.TargetOvers))
	return describeTarget(mi)
}
