package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// These integration tests guard against regressions that occurred during cricsheet import:
// 1. "relation X_tmp already exists" when the same transaction runs multiple batches (e.g. two innings).
// 2. "conn busy" when RecomputeFieldingAggregatesTx runs a Query then uses the same tx for UpsertFieldingTx
//    before the query rows are fully consumed and closed.
//
// Run with: RUN_DB_TESTS=1 go test -v ./internal/db -run TestMultiBatch|TestRecomputeFieldingAggregatesTx

func TestMultiBatchFieldingEventsInSameTx(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	if err := RunMigrations(ctx, migrationsDir()); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	tx, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	// First batch (inning 1)
	batch1 := []FieldingEvent{
		{MatchID: 888881, Innings: 1, Over: 0, Ball: 1, Kind: "caught", AssistRole: ""},
	}
	if err := InsertFieldingEventsBatchTx(ctx, tx, batch1); err != nil {
		t.Fatalf("first InsertFieldingEventsBatchTx: %v", err)
	}

	// Second batch (inning 2) in same tx — would fail with "relation \"fielding_event_tmp\" already exists" without DROP TABLE IF EXISTS
	batch2 := []FieldingEvent{
		{MatchID: 888881, Innings: 2, Over: 0, Ball: 1, Kind: "run_out", AssistRole: ""},
	}
	if err := InsertFieldingEventsBatchTx(ctx, tx, batch2); err != nil {
		t.Fatalf("second InsertFieldingEventsBatchTx: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestMultiBatchBattingInSameTx(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	if err := RunMigrations(ctx, migrationsDir()); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	// Ensure we have at least one player (batting_data has FK to player)
	_ = PoolAPI.Exec(
		ctx,
		"INSERT INTO player(player_name) VALUES ('TestBatchPlayer') ON CONFLICT (player_name) DO NOTHING",
	)
	var playerID int64
	if err := PoolAPI.QueryRow(ctx, "SELECT id FROM player WHERE player_name = 'TestBatchPlayer'").Scan(&playerID); err != nil {
		t.Fatalf("select test player: %v", err)
	}

	tx, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	runs1, runs2 := 10, 20
	batch1 := []Batting{
		{MatchID: 888882, InningNumber: 1, PlayerID: playerID, Runs: &runs1},
	}
	if err := UpsertBattingBatchTx(ctx, tx, batch1); err != nil {
		t.Fatalf("first UpsertBattingBatchTx: %v", err)
	}

	batch2 := []Batting{
		{MatchID: 888882, InningNumber: 2, PlayerID: playerID, Runs: &runs2},
	}
	if err := UpsertBattingBatchTx(ctx, tx, batch2); err != nil {
		t.Fatalf("second UpsertBattingBatchTx: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestMultiBatchBowlingInSameTx(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	if err := RunMigrations(ctx, migrationsDir()); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	_ = PoolAPI.Exec(
		ctx,
		"INSERT INTO player(player_name) VALUES ('TestBowlBatchPlayer') ON CONFLICT (player_name) DO NOTHING",
	)
	var playerID int64
	if err := PoolAPI.QueryRow(ctx, "SELECT id FROM player WHERE player_name = 'TestBowlBatchPlayer'").Scan(&playerID); err != nil {
		t.Fatalf("select test player: %v", err)
	}

	tx, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	wickets1, wickets2 := 1, 2
	batch1 := []Bowling{
		{MatchID: 888883, InningNumber: 1, PlayerID: playerID, Wickets: &wickets1},
	}
	if err := UpsertBowlingBatchTx(ctx, tx, batch1); err != nil {
		t.Fatalf("first UpsertBowlingBatchTx: %v", err)
	}

	batch2 := []Bowling{
		{MatchID: 888883, InningNumber: 2, PlayerID: playerID, Wickets: &wickets2},
	}
	if err := UpsertBowlingBatchTx(ctx, tx, batch2); err != nil {
		t.Fatalf("second UpsertBowlingBatchTx: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestRecomputeFieldingAggregatesTxNoConnBusy(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	if err := RunMigrations(ctx, migrationsDir()); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	// Run in a single transaction: insert player, insert fielding_events, then RecomputeFieldingAggregatesTx.
	// Without consuming and closing the aggregation query rows before calling UpsertFieldingTx, we get "conn busy".
	err = RunInTx(ctx, func(ctx context.Context, tx CopyFromTx) error {
		_ = tx.Exec(
			ctx,
			"INSERT INTO player(player_name) VALUES ('TestRecomputeFielder') ON CONFLICT (player_name) DO NOTHING",
		)
		var playerID int64
		if err := tx.QueryRow(ctx, "SELECT id FROM player WHERE player_name = 'TestRecomputeFielder'").Scan(&playerID); err != nil {
			return err
		}

		// Insert fielding events so the aggregation query returns rows
		events := []FieldingEvent{
			{MatchID: 888884, Innings: 1, Over: 0, Ball: 1, FielderID: &playerID, Kind: "caught", AssistRole: ""},
			{MatchID: 888884, Innings: 1, Over: 0, Ball: 2, FielderID: &playerID, Kind: "caught", AssistRole: ""},
		}
		if err := InsertFieldingEventsBatchTx(ctx, tx, events); err != nil {
			return err
		}

		// RecomputeFieldingAggregatesTx does a Query then UpsertFieldingTx per row; rows must be fully read and closed first to avoid "conn busy"
		return RecomputeFieldingAggregatesTx(ctx, tx, 888884)
	})
	require.NoError(t, err)
}
