package db

import (
	"context"
	"os"
	"testing"
)

// guardIntegration returns true if RUN_DB_TESTS=1
func guardIntegration(t *testing.T) bool {
	t.Helper()
	return os.Getenv("RUN_DB_TESTS") == "1"
}

func TestInsertBallEvents_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Run migrations from go-app/migrations
	if err := RunMigrations(ctx, "./go-app/migrations"); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	// Clean table
	if err := Exec(ctx, "TRUNCATE ball_event"); err != nil {
		t.Fatalf("truncate failed: %v", err)
	}

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
	if err := InsertBallEvents(ctx, rows); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	// Count rows
	var cnt int
	if err := QueryRow(ctx, "SELECT count(*) FROM ball_event").Scan(&cnt); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if cnt != len(rows) {
		t.Fatalf("unexpected count after first insert: got %d want %d", cnt, len(rows))
	}

	// Re-insert same rows (idempotent)
	if err := InsertBallEvents(ctx, rows); err != nil {
		t.Fatalf("second insert (duplicates) failed: %v", err)
	}
	if err := QueryRow(ctx, "SELECT count(*) FROM ball_event").Scan(&cnt); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if cnt != len(rows) {
		t.Fatalf("unexpected count after duplicate insert: got %d want %d", cnt, len(rows))
	}

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
	if err := InsertBallEvents(ctx, more); err != nil {
		t.Fatalf("mixed insert failed: %v", err)
	}
	if err := QueryRow(ctx, "SELECT count(*) FROM ball_event").Scan(&cnt); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if cnt != len(rows)+1 {
		t.Fatalf("unexpected count after mixed insert: got %d want %d", cnt, len(rows)+1)
	}
}
