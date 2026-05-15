package db

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// guardIntegration returns true if RUN_DB_TESTS=1
func guardIntegration(t *testing.T) bool {
	t.Helper()
	return os.Getenv("RUN_DB_TESTS") == "1"
}

// migrationsDir returns an absolute path to the migrations directory regardless of the
// working directory Go test uses (which may be a temp dir). It derives the path from
// this test file's location.
func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	// This test file is in go-app/internal/db; migrations live in go-app/migrations
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../migrations"))
}

func TestInsertBallEvents_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	// Run migrations using an absolute path derived from this test file
	require.NoError(t, RunMigrations(ctx, migrationsDir()))

	// Clean table
	require.NoError(t, Exec(ctx, "TRUNCATE ball_event"))

	// Prepare rows
	rows := []BallEventRow{
		{
			MatchID:    1,
			Innings:    1,
			Over:       1,
			Ball:       1,
			BallSeq:    1,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 0,
			RunsExtras: 0,
			RunsTotal:  0,
		},
		{
			MatchID:    1,
			Innings:    1,
			Over:       1,
			Ball:       2,
			BallSeq:    2,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 1,
			RunsExtras: 0,
			RunsTotal:  1,
		},
		{
			MatchID:    1,
			Innings:    1,
			Over:       1,
			Ball:       3,
			BallSeq:    3,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 4,
			RunsExtras: 0,
			RunsTotal:  4,
		},
		{
			MatchID:    1,
			Innings:    1,
			Over:       1,
			Ball:       4,
			BallSeq:    4,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 0,
			RunsExtras: 1,
			RunsTotal:  1,
		},
		{
			MatchID:    1,
			Innings:    1,
			Over:       1,
			Ball:       5,
			BallSeq:    5,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 0,
			RunsExtras: 0,
			RunsTotal:  0,
		},
		{
			MatchID:    1,
			Innings:    1,
			Over:       1,
			Ball:       6,
			BallSeq:    6,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 2,
			RunsExtras: 0,
			RunsTotal:  2,
		},
		{
			MatchID:    1,
			Innings:    1,
			Over:       2,
			Ball:       1,
			BallSeq:    7,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 0,
			RunsExtras: 0,
			RunsTotal:  0,
		},
		{
			MatchID:    1,
			Innings:    1,
			Over:       2,
			Ball:       2,
			BallSeq:    8,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 0,
			RunsExtras: 0,
			RunsTotal:  0,
		},
		{
			MatchID:    1,
			Innings:    1,
			Over:       2,
			Ball:       3,
			BallSeq:    9,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 3,
			RunsExtras: 0,
			RunsTotal:  3,
		},
	}

	// First insert
	require.NoError(t, InsertBallEvents(ctx, rows))

	// Count rows
	var cnt int
	require.NoError(t, QueryRow(ctx, "SELECT count(*) FROM ball_event").Scan(&cnt))
	require.Equal(t, len(rows), cnt)

	// Re-insert same rows (idempotent)
	require.NoError(t, InsertBallEvents(ctx, rows))
	require.NoError(t, QueryRow(ctx, "SELECT count(*) FROM ball_event").Scan(&cnt))
	require.Equal(t, len(rows), cnt)

	// Insert mix of duplicates + one new
	more := append([]BallEventRow{}, rows...)
	more = append(
		more,
		BallEventRow{
			MatchID:    1,
			Innings:    1,
			Over:       2,
			Ball:       4,
			BallSeq:    10,
			IsLegal:    true,
			Phase:      "pp",
			RunsBatter: 1,
			RunsExtras: 0,
			RunsTotal:  1,
		},
	)
	require.NoError(t, InsertBallEvents(ctx, more))
	require.NoError(t, QueryRow(ctx, "SELECT count(*) FROM ball_event").Scan(&cnt))
	require.Equal(t, len(rows)+1, cnt)
}
