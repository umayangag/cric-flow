package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// FieldingEvent represents a row in fielding_event.
// Optional player references are pointers; nil means unresolved.
type FieldingEvent struct {
	MatchID     int64
	Innings     int
	Over        int
	Ball        int
	BatterOutID *int64
	FielderID   *int64
	BowlerID    *int64
	Kind        string
	AssistRole  string // "", "assist", "keeper", or "primary" (reserved)
	IsDirectHit bool
	Notes       *string
}

// InsertFieldingEvent inserts a fielding_event row idempotently using the natural
// unique key (match_id, innings, over, ball, fielder_id, kind, assist_role).
func InsertFieldingEvent(ctx context.Context, e *FieldingEvent) error {
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	// assist_role is stored as empty string if not provided for idempotency
	err := PoolAPI.Exec(ctx, `
        INSERT INTO fielding_event(
            match_id, innings, over, ball, batter_out_id, fielder_id, bowler_id,
            kind, assist_role, is_direct_hit, notes
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9,''),$10,$11)
        ON CONFLICT (match_id, innings, over, ball, fielder_id, kind, assist_role) DO NOTHING
    `, e.MatchID, e.Innings, e.Over, e.Ball, e.BatterOutID, e.FielderID, e.BowlerID, e.Kind, e.AssistRole, e.IsDirectHit, e.Notes)
	return err
}

// RecomputeFieldingAggregates aggregates fielding_event into fielding_data for a match.
// It counts catches, run outs, stumpings, and direct-hit run outs per fielder.
func RecomputeFieldingAggregates(ctx context.Context, matchID int64) error {
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	// Aggregate counts per fielder for the given match; exclude NULL fielder_id
	rows, err := PoolAPI.Query(ctx, `
        WITH base AS (
            SELECT
                fe.fielder_id AS player_id,
                SUM(CASE WHEN fe.kind = 'caught' THEN 1 ELSE 0 END) AS catches,
                SUM(CASE WHEN fe.kind = 'run_out' THEN 1 ELSE 0 END) AS run_outs,
                SUM(CASE WHEN fe.kind = 'stumped' THEN 1 ELSE 0 END) AS stumpings,
                SUM(CASE WHEN fe.kind = 'run_out' AND fe.is_direct_hit THEN 1 ELSE 0 END) AS runouts_direct_hits
            FROM fielding_event fe
            WHERE fe.match_id = $1 AND fe.fielder_id IS NOT NULL
            GROUP BY fe.fielder_id
        )
        SELECT player_id, catches, run_outs, stumpings, runouts_direct_hits FROM base
    `, matchID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var playerID int64
		var catches, runOuts, stumpings, directHits int
		if err := rows.Scan(&playerID, &catches, &runOuts, &stumpings, &directHits); err != nil {
			return err
		}
		// Upsert aggregates into fielding_data. Ensure zero/NULL safety using pointers.
		c, r, s, dh := catches, runOuts, stumpings, directHits
		// Keep existing dropped/missed as-is by passing nils; UpsertFielding handles COALESCE
		if err := UpsertFielding(ctx, &Fielding{
			MatchID:           matchID,
			PlayerID:          playerID,
			Catches:           &c,
			RunOuts:           &r,
			DroppedCatches:    nil,
			MissedRunOuts:     nil,
			Stumpings:         &s,
			RunoutsDirectHits: &dh,
		}); err != nil {
			return err
		}
	}
	return nil
}

// RecomputeFieldingAggregatesTx aggregates fielding_event into fielding_data for a match using the given transaction.
// Results are read into a slice first so the connection is not busy when calling UpsertFieldingTx (pgx does not allow concurrent use).
func RecomputeFieldingAggregatesTx(ctx context.Context, tx CopyFromTx, matchID int64) error {
	rows, err := tx.Query(ctx, `
        WITH base AS (
            SELECT
                fe.fielder_id AS player_id,
                SUM(CASE WHEN fe.kind = 'caught' THEN 1 ELSE 0 END) AS catches,
                SUM(CASE WHEN fe.kind = 'run_out' THEN 1 ELSE 0 END) AS run_outs,
                SUM(CASE WHEN fe.kind = 'stumped' THEN 1 ELSE 0 END) AS stumpings,
                SUM(CASE WHEN fe.kind = 'run_out' AND fe.is_direct_hit THEN 1 ELSE 0 END) AS runouts_direct_hits
            FROM fielding_event fe
            WHERE fe.match_id = $1 AND fe.fielder_id IS NOT NULL
            GROUP BY fe.fielder_id
        )
        SELECT player_id, catches, run_outs, stumpings, runouts_direct_hits FROM base
    `, matchID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var results []struct {
		playerID   int64
		catches    int
		runOuts    int
		stumpings  int
		directHits int
	}
	for rows.Next() {
		var r struct {
			playerID   int64
			catches    int
			runOuts    int
			stumpings  int
			directHits int
		}
		if err := rows.Scan(&r.playerID, &r.catches, &r.runOuts, &r.stumpings, &r.directHits); err != nil {
			return err
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close() // release connection before using tx again

	fieldingRows := make([]Fielding, 0, len(results))
	for _, r := range results {
		c, ro, s, dh := r.catches, r.runOuts, r.stumpings, r.directHits
		fieldingRows = append(fieldingRows, Fielding{
			MatchID:           matchID,
			PlayerID:          r.playerID,
			Catches:           &c,
			RunOuts:           &ro,
			DroppedCatches:    nil,
			MissedRunOuts:     nil,
			Stumpings:         &s,
			RunoutsDirectHits: &dh,
		})
	}
	if err := UpsertFieldingBatchTx(ctx, tx, fieldingRows); err != nil {
		return err
	}
	return nil
}

// InsertFieldingEventsBatch inserts multiple fielding_event rows idempotently using pgx.CopyFrom and a temporary table.
func InsertFieldingEventsBatch(ctx context.Context, rows []FieldingEvent) error {
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	if len(rows) == 0 {
		return nil
	}

	// Use a temporary table and then INSERT ... ON CONFLICT DO NOTHING for an atomic and performant bulk insert.
	tx, err := PoolAPI.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Create a temporary table with the same structure.
	err = tx.Exec(ctx, `CREATE TEMP TABLE fielding_event_tmp (LIKE fielding_event INCLUDING DEFAULTS) ON COMMIT DROP`)
	if err != nil {
		return fmt.Errorf("create temp table: %w", err)
	}

	// 2. Use CopyFrom to bulk insert into the temporary table.
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"fielding_event_tmp"},
		[]string{
			"match_id",
			"innings",
			"over",
			"ball",
			"batter_out_id",
			"fielder_id",
			"bowler_id",
			"kind",
			"assist_role",
			"is_direct_hit",
			"notes",
		},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			// assist_role is stored as empty string if not provided for idempotency
			role := r.AssistRole
			if role == "" {
				role = "" // Already empty string
			}
			return []any{
				r.MatchID,
				r.Innings,
				r.Over,
				r.Ball,
				r.BatterOutID,
				r.FielderID,
				r.BowlerID,
				r.Kind,
				role,
				r.IsDirectHit,
				r.Notes,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy from: %w", err)
	}

	// 3. Insert from the temporary table into the main table idempotently.
	err = tx.Exec(ctx, `
		INSERT INTO fielding_event (match_id, innings, over, ball, batter_out_id, fielder_id, bowler_id, kind, assist_role, is_direct_hit, notes)
		SELECT match_id, innings, over, ball, batter_out_id, fielder_id, bowler_id, kind, COALESCE(assist_role, ''), is_direct_hit, notes
		FROM fielding_event_tmp
		ON CONFLICT (match_id, innings, over, ball, fielder_id, kind, assist_role) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("merge temp table: %w", err)
	}

	return tx.Commit(ctx)
}

// InsertFieldingEventsBatchTx inserts multiple fielding_event rows using the given transaction.
// The same transaction may be used for multiple batches (e.g. one per inning), so we drop the temp table if it exists.
func InsertFieldingEventsBatchTx(ctx context.Context, tx CopyFromTx, rows []FieldingEvent) error {
	if len(rows) == 0 {
		return nil
	}
	_ = tx.Exec(ctx, `DROP TABLE IF EXISTS fielding_event_tmp`)
	err := tx.Exec(ctx, `CREATE TEMP TABLE fielding_event_tmp (LIKE fielding_event INCLUDING DEFAULTS) ON COMMIT DROP`)
	if err != nil {
		return fmt.Errorf("create temp table: %w", err)
	}
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"fielding_event_tmp"},
		[]string{
			"match_id", "innings", "over", "ball", "batter_out_id", "fielder_id", "bowler_id",
			"kind", "assist_role", "is_direct_hit", "notes",
		},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			role := r.AssistRole
			return []any{
				r.MatchID, r.Innings, r.Over, r.Ball, r.BatterOutID, r.FielderID, r.BowlerID,
				r.Kind, role, r.IsDirectHit, r.Notes,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy from: %w", err)
	}
	return tx.Exec(ctx, `
		INSERT INTO fielding_event (match_id, innings, over, ball, batter_out_id, fielder_id, bowler_id, kind, assist_role, is_direct_hit, notes)
		SELECT match_id, innings, over, ball, batter_out_id, fielder_id, bowler_id, kind, COALESCE(assist_role, ''), is_direct_hit, notes
		FROM fielding_event_tmp
		ON CONFLICT (match_id, innings, over, ball, fielder_id, kind, assist_role) DO NOTHING
	`)
}
