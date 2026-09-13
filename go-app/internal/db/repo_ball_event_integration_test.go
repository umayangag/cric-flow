package db

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// migrationsDir returns an absolute path to the migrations directory regardless of the
// working directory Go test uses (which may be a temp dir). It derives the path from
// this test file's location.
func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	// This test file is in go-app/internal/db; migrations live in go-app/migrations
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../migrations"))
}

// ballEventRowsForMatch builds a run of deliveries in one over of one innings, each with
// runsTotal off the bat, so a test can tell one version of a match from another by the
// runs stored against a delivery.
func ballEventRowsForMatch(matchID int64, deliveries int, runsTotal int) []BallEventRow {
	rows := make([]BallEventRow, 0, deliveries)
	for i := 0; i < deliveries; i++ {
		rows = append(rows, BallEventRow{
			MatchID:    matchID,
			Innings:    1,
			Over:       1 + i/6,
			Ball:       1 + i%6,
			BallSeq:    1 + i,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: runsTotal,
			RunsTotal:  runsTotal,
		})
	}
	return rows
}

// TestInsertBallEventsTx_ReImportOfAMatch_ReplacesEveryRow pins the SQL half of IMPORT-03.
//
// The insert used to carry ON CONFLICT (match_id, innings, over, ball) DO NOTHING, so a
// second import of the same match wrote nothing and a correction to a delivery never
// landed. It is now a plain insert, and DeleteMatchFactsTx is what makes room for the new
// rows -- which is exactly what the two halves of this test assert: without the delete the
// insert fails on the primary key, and with it the match reads as the new version alone.
func TestInsertBallEventsTx_ReImportOfAMatch_ReplacesEveryRow(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, RunMigrations(ctx, migrationsDir()))
	require.NoError(t, Exec(ctx, "TRUNCATE ball_event_wicket, ball_event"))

	const matchID = int64(1)
	// Nine deliveries take the bulk COPY path (smallBatchThreshold is 8); the six below
	// take the per-row path, so the re-import crosses both.
	first := ballEventRowsForMatch(matchID, 9, 1)
	second := ballEventRowsForMatch(matchID, 6, 4)

	// Arrange: the match as a first import left it.
	tx, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, InsertBallEventsTx(ctx, tx, first))
	require.NoError(t, tx.Commit(ctx))
	var count int
	require.NoError(t, QueryRow(ctx, "SELECT count(*) FROM ball_event").Scan(&count))
	require.Equal(t, len(first), count)

	// Act and assert: inserting over an imported match without clearing it first is a
	// primary-key violation now, not a silent no-op.
	txWithoutDelete, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	require.Error(t, InsertBallEventsTx(ctx, txWithoutDelete, second))
	require.NoError(t, txWithoutDelete.Rollback(ctx))

	// Act: the re-import, as the importer runs it.
	txReimport, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, DeleteMatchFactsTx(ctx, txReimport, matchID))
	require.NoError(t, InsertBallEventsTx(ctx, txReimport, second))
	require.NoError(t, txReimport.Commit(ctx))

	// Assert: the new version of the match, and nothing of the old one.
	require.NoError(t, QueryRow(ctx, "SELECT count(*) FROM ball_event").Scan(&count))
	require.Equal(t, len(second), count)
	var runsOnFirstBall int
	require.NoError(t, QueryRow(
		ctx,
		`SELECT runs_total FROM ball_event WHERE match_id = $1 AND innings = 1 AND "over" = 1 AND ball = 1`,
		matchID,
	).Scan(&runsOnFirstBall))
	require.Equal(t, 4, runsOnFirstBall)
}
