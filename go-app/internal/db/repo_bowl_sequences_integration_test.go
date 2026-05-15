package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpsertBowlingSequences_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	// Run migrations using an absolute path derived from this test package
	require.NoError(t, RunMigrations(ctx, migrationsDir()))

	// Clean table
	require.NoError(t, Exec(ctx, "TRUNCATE bowling_sequence_features"))

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

	require.NoError(t, UpsertBowlingSequences(ctx, base))

	// Verify count
	var cnt int
	require.NoError(t, QueryRow(ctx, `SELECT count(*) FROM bowling_sequence_features`).Scan(&cnt))
	require.Equal(t, len(base), cnt)

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
	require.NoError(t, UpsertBowlingSequences(ctx, []BowlSequenceRow{upd}))

	// Verify row count unchanged
	require.NoError(t, QueryRow(ctx, `SELECT count(*) FROM bowling_sequence_features`).Scan(&cnt))
	require.Equal(t, len(base), cnt)

	// Verify the updated values
	var oversPairs, balls, runs, wickets, dotBalls int
	require.NoError(t, QueryRow(ctx, `
        SELECT overs_pairs, balls, runs, wickets, dot_balls
        FROM bowling_sequence_features
        WHERE as_of_date = $1 AND format_id = $2 AND scope = $3 AND scope_id IS NULL
          AND prev_bowler_id = $4 AND bowler_id = $5 AND phase = $6
    `, upd.AsOfDate, upd.FormatID, "overall", upd.PrevBowlerID, upd.BowlerID, upd.Phase).Scan(&oversPairs, &balls, &runs, &wickets, &dotBalls))
	require.Equal(t, upd.OversPairs, oversPairs)
	require.Equal(t, upd.Balls, balls)
	require.Equal(t, upd.Runs, runs)
	require.Equal(t, upd.Wickets, wickets)
	require.Equal(t, upd.DotBalls, dotBalls)
}
