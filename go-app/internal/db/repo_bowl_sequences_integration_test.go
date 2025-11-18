package db

import (
	"context"
	"testing"
)

func TestUpsertBowlingSequences_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Run migrations using an absolute path derived from this test package
	if err := RunMigrations(ctx, migrationsDir()); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	// Clean table
	if err := Exec(ctx, "TRUNCATE bowling_sequence_features"); err != nil {
		t.Fatalf("truncate failed: %v", err)
	}

	// Prepare an initial batch (> smallBatchThreshold to exercise COPY path)
	base := []BowlSequenceRow{
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 101,
			BowlerID:     201,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         4,
			Wickets:      0,
			DotBalls:     3,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 201,
			BowlerID:     301,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         2,
			Wickets:      1,
			DotBalls:     4,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 301,
			BowlerID:     401,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         8,
			Wickets:      0,
			DotBalls:     2,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 401,
			BowlerID:     501,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         5,
			Wickets:      0,
			DotBalls:     3,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 501,
			BowlerID:     601,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         3,
			Wickets:      1,
			DotBalls:     4,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 601,
			BowlerID:     701,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         7,
			Wickets:      0,
			DotBalls:     1,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 701,
			BowlerID:     801,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         6,
			Wickets:      2,
			DotBalls:     2,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 801,
			BowlerID:     901,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         1,
			Wickets:      0,
			DotBalls:     5,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: 901,
			BowlerID:     1001,
			Phase:        "pp",
			OversPairs:   1,
			Balls:        6,
			Runs:         9,
			Wickets:      0,
			DotBalls:     1,
		},
	}

	if err := UpsertBowlingSequences(ctx, base); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	// Verify count
	var cnt int
	if err := QueryRow(ctx, `SELECT count(*) FROM bowling_sequence_features`).Scan(&cnt); err != nil {
		t.Fatalf("count scan failed: %v", err)
	}
	if cnt != len(base) {
		t.Fatalf("unexpected row count: got %d want %d", cnt, len(base))
	}

	// Upsert a conflicting row with new values to test ON CONFLICT DO UPDATE
	upd := BowlSequenceRow{
		AsOfDate:     "2024-01-01",
		FormatID:     1,
		Scope:        "overall",
		ScopeID:      nil,
		PrevBowlerID: 101,
		BowlerID:     201,
		Phase:        "pp",
		OversPairs:   2,
		Balls:        12,
		Runs:         10,
		Wickets:      1,
		DotBalls:     6,
	}
	if err := UpsertBowlingSequences(ctx, []BowlSequenceRow{upd}); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	// Verify row count unchanged
	if err := QueryRow(ctx, `SELECT count(*) FROM bowling_sequence_features`).Scan(&cnt); err != nil {
		t.Fatalf("count rescan failed: %v", err)
	}
	if cnt != len(base) {
		t.Fatalf("row count changed after update: got %d want %d", cnt, len(base))
	}

	// Verify the updated values
	var oversPairs, balls, runs, wickets, dotBalls int
	if err := QueryRow(ctx, `
        SELECT overs_pairs, balls, runs, wickets, dot_balls
        FROM bowling_sequence_features
        WHERE as_of_date = $1 AND format_id = $2 AND scope = $3 AND scope_id IS NULL
          AND prev_bowler_id = $4 AND bowler_id = $5 AND phase = $6
    `, upd.AsOfDate, upd.FormatID, "overall", upd.PrevBowlerID, upd.BowlerID, upd.Phase).Scan(&oversPairs, &balls, &runs, &wickets, &dotBalls); err != nil {
		t.Fatalf("select updated row failed: %v", err)
	}
	if oversPairs != upd.OversPairs || balls != upd.Balls || runs != upd.Runs || wickets != upd.Wickets ||
		dotBalls != upd.DotBalls {
		t.Fatalf("updated values mismatch: got (op=%d b=%d r=%d w=%d d=%d) want (op=%d b=%d r=%d w=%d d=%d)",
			oversPairs, balls, runs, wickets, dotBalls, upd.OversPairs, upd.Balls, upd.Runs, upd.Wickets, upd.DotBalls)
	}
}
