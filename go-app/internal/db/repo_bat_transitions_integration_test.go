package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpsertBattingTransitions_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	// Run migrations using an absolute path derived from this test package
	if err := RunMigrations(ctx, migrationsDir()); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	// Clean table
	if err := Exec(ctx, "TRUNCATE batting_transition_features"); err != nil {
		t.Fatalf("truncate failed: %v", err)
	}

	// Prepare an initial batch (> smallBatchThreshold to exercise COPY path)
	base := []BatTransitionRow{
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 10,
			BatterID:     11,
			Phase:        "pp",
			Balls:        10,
			Runs:         12,
			Dismissals:   1,
			Fours:        2,
			Sixes:        1,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 11,
			BatterID:     12,
			Phase:        "pp",
			Balls:        8,
			Runs:         9,
			Dismissals:   0,
			Fours:        1,
			Sixes:        0,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 12,
			BatterID:     13,
			Phase:        "pp",
			Balls:        6,
			Runs:         7,
			Dismissals:   0,
			Fours:        1,
			Sixes:        1,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 13,
			BatterID:     14,
			Phase:        "pp",
			Balls:        5,
			Runs:         3,
			Dismissals:   0,
			Fours:        0,
			Sixes:        0,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 14,
			BatterID:     15,
			Phase:        "pp",
			Balls:        9,
			Runs:         10,
			Dismissals:   1,
			Fours:        1,
			Sixes:        0,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 15,
			BatterID:     16,
			Phase:        "pp",
			Balls:        7,
			Runs:         4,
			Dismissals:   0,
			Fours:        0,
			Sixes:        0,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 16,
			BatterID:     17,
			Phase:        "pp",
			Balls:        11,
			Runs:         13,
			Dismissals:   1,
			Fours:        2,
			Sixes:        0,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 17,
			BatterID:     18,
			Phase:        "pp",
			Balls:        4,
			Runs:         2,
			Dismissals:   0,
			Fours:        0,
			Sixes:        0,
		},
		{
			AsOfDate:     "2024-01-01",
			FormatID:     1,
			Scope:        "overall",
			ScopeID:      0,
			PrevBatterID: 18,
			BatterID:     19,
			Phase:        "pp",
			Balls:        12,
			Runs:         15,
			Dismissals:   1,
			Fours:        3,
			Sixes:        0,
		},
	}

	if err := UpsertBattingTransitions(ctx, base); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	// Verify count
	var cnt int
	if err := QueryRow(ctx, `SELECT count(*) FROM batting_transition_features`).Scan(&cnt); err != nil {
		t.Fatalf("count scan failed: %v", err)
	}
	if cnt != len(base) {
		t.Fatalf("unexpected row count: got %d want %d", cnt, len(base))
	}

	// Upsert a conflicting row with new values to test ON CONFLICT DO UPDATE
	upd := BatTransitionRow{
		AsOfDate:     "2024-01-01",
		FormatID:     1,
		Scope:        "overall",
		ScopeID:      0,
		PrevBatterID: 10,
		BatterID:     11,
		Phase:        "pp",
		Balls:        20,
		Runs:         30,
		Dismissals:   2,
		Fours:        4,
		Sixes:        2,
	}
	if err := UpsertBattingTransitions(ctx, []BatTransitionRow{upd}); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	// Verify row count unchanged
	if err := QueryRow(ctx, `SELECT count(*) FROM batting_transition_features`).Scan(&cnt); err != nil {
		t.Fatalf("count rescan failed: %v", err)
	}
	if cnt != len(base) {
		t.Fatalf("row count changed after update: got %d want %d", cnt, len(base))
	}

	// Verify the updated values
	var balls, runs, dismissals, fours, sixes int
	if err := QueryRow(ctx, `
        SELECT balls, runs, dismissals, fours, sixes
        FROM batting_transition_features
        WHERE as_of_date = $1 AND format_id = $2 AND scope = $3 AND scope_id = $4
          AND prev_batter_id = $5 AND batter_id = $6 AND phase = $7
    `, upd.AsOfDate, upd.FormatID, "overall", 0, upd.PrevBatterID, upd.BatterID, upd.Phase).Scan(&balls, &runs, &dismissals, &fours, &sixes); err != nil {
		t.Fatalf("select updated row failed: %v", err)
	}
	if balls != upd.Balls || runs != upd.Runs || dismissals != upd.Dismissals || fours != upd.Fours ||
		sixes != upd.Sixes {
		t.Fatalf("updated values mismatch: got (b=%d r=%d d=%d f=%d s=%d) want (b=%d r=%d d=%d f=%d s=%d)",
			balls, runs, dismissals, fours, sixes, upd.Balls, upd.Runs, upd.Dismissals, upd.Fours, upd.Sixes)
	}
}
