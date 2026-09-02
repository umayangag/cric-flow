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

// namedPlayerID returns the id of a player keyed by name alone, creating the row if it is
// new. batting_data, bowling_data and fielding_event all have a foreign key to player, so
// each of these tests needs one before it can insert anything.
//
// It goes through GetOrCreatePlayer rather than its own INSERT. Each of these tests used to
// carry a copy of that statement, and the copies went stale: migration 0004 made the
// player_name unique index *partial* (`WHERE external_id IS NULL`, because a registry id is
// the identity where there is one), so `ON CONFLICT (player_name)` stopped naming a
// constraint and the insert began erroring. The error went to `_`, the following SELECT
// found no row, and three tests failed on a line that looked nothing like the cause. A test
// fixture that duplicates production SQL is a copy that can rot; this one cannot.
func namedPlayerID(ctx context.Context, t *testing.T, name string) int64 {
	t.Helper()
	id, _, err := GetOrCreatePlayer(ctx, "", name, "")
	require.NoError(t, err)
	return id
}

func TestMultiBatchFieldingEventsInSameTx(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	require.NoError(t, RunMigrations(ctx, migrationsDir()))

	tx, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	// First batch (inning 1)
	batch1 := []FieldingEvent{
		{MatchID: 888881, Innings: 1, Over: 0, Ball: 1, Kind: "caught", AssistRole: ""},
	}
	require.NoError(t, InsertFieldingEventsBatchTx(ctx, tx, batch1))

	// Second batch (inning 2) in same tx — would fail with "relation \"fielding_event_tmp\" already exists" without DROP TABLE IF EXISTS
	batch2 := []FieldingEvent{
		{MatchID: 888881, Innings: 2, Over: 0, Ball: 1, Kind: "run_out", AssistRole: ""},
	}
	require.NoError(t, InsertFieldingEventsBatchTx(ctx, tx, batch2))

	require.NoError(t, tx.Commit(ctx))
}

func TestMultiBatchBattingInSameTx(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	require.NoError(t, RunMigrations(ctx, migrationsDir()))

	playerID := namedPlayerID(ctx, t, "TestBatchPlayer")

	tx, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	runs1, runs2 := 10, 20
	batch1 := []Batting{
		{MatchID: 888882, InningNumber: 1, PlayerID: playerID, Runs: &runs1},
	}
	require.NoError(t, UpsertBattingBatchTx(ctx, tx, batch1))

	batch2 := []Batting{
		{MatchID: 888882, InningNumber: 2, PlayerID: playerID, Runs: &runs2},
	}
	require.NoError(t, UpsertBattingBatchTx(ctx, tx, batch2))

	require.NoError(t, tx.Commit(ctx))
}

func TestMultiBatchBowlingInSameTx(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	require.NoError(t, RunMigrations(ctx, migrationsDir()))

	playerID := namedPlayerID(ctx, t, "TestBowlBatchPlayer")

	tx, err := PoolAPI.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	wickets1, wickets2 := 1, 2
	batch1 := []Bowling{
		{MatchID: 888883, InningNumber: 1, PlayerID: playerID, Wickets: &wickets1},
	}
	require.NoError(t, UpsertBowlingBatchTx(ctx, tx, batch1))

	batch2 := []Bowling{
		{MatchID: 888883, InningNumber: 2, PlayerID: playerID, Wickets: &wickets2},
	}
	require.NoError(t, UpsertBowlingBatchTx(ctx, tx, batch2))

	require.NoError(t, tx.Commit(ctx))
}

func TestRecomputeFieldingAggregatesTxNoConnBusy(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	require.NoError(t, RunMigrations(ctx, migrationsDir()))

	// The fielder exists before the transaction: he is a foreign key this test needs, not
	// part of the regression it guards.
	playerID := namedPlayerID(ctx, t, "TestRecomputeFielder")

	// Run in a single transaction: insert fielding_events, then RecomputeFieldingAggregatesTx.
	// Without consuming and closing the aggregation query rows before calling UpsertFieldingTx, we get "conn busy".
	err = RunInTx(ctx, func(ctx context.Context, tx CopyFromTx) error {
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
