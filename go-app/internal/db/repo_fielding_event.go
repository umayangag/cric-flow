package db

import (
	"context"
	"errors"
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

// InsertFieldingEventsBatch inserts multiple fielding_event rows idempotently.
func InsertFieldingEventsBatch(ctx context.Context, rows []FieldingEvent) error {
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	if len(rows) == 0 {
		return nil
	}
	// Use a transaction for the batch
	tx, err := PoolAPI.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, e := range rows {
		// assist_role is stored as empty string if not provided for idempotency
		err := tx.Exec(ctx, `
            INSERT INTO fielding_event(
                match_id, innings, over, ball, batter_out_id, fielder_id, bowler_id,
                kind, assist_role, is_direct_hit, notes
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9,''),$10,$11)
            ON CONFLICT (match_id, innings, over, ball, fielder_id, kind, assist_role) DO NOTHING
        `, e.MatchID, e.Innings, e.Over, e.Ball, e.BatterOutID, e.FielderID, e.BowlerID, e.Kind, e.AssistRole, e.IsDirectHit, e.Notes)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
