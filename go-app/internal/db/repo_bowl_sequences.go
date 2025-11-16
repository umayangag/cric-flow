package db

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// BowlSequenceRow mirrors columns for insertion/upsert into bowling_sequence_features.
// scope is currently 'overall' with NULL scope_id per plan; scope_id0 is generated in the table.
type BowlSequenceRow struct {
	AsOfDate     string // YYYY-MM-DD
	FormatID     int
	Scope        string // 'overall'
	ScopeID      *int64 // nil for overall
	PrevBowlerID int64
	BowlerID     int64
	Phase        string
	OversPairs   int
	Balls        int
	Runs         int
	Wickets      int
	DotBalls     int
}

// UpsertBowlingSequences performs idempotent upserts for bowling_sequence_features rows.
func UpsertBowlingSequences(ctx context.Context, rows []BowlSequenceRow) error {
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	if len(rows) == 0 {
		slog.Warn("[UpsertBowlingSequences] no rows to upsert")
		return nil
	}

	// For small batches the COPY overhead can outweigh benefits.
	const smallBatchThreshold = 8
	if len(rows) <= smallBatchThreshold {
		for i := range rows {
			r := rows[i]
			// Normalize defaults: scope defaults to 'overall'; overall uses NULL scope_id
			scope := r.Scope
			if scope == "" {
				scope = "overall"
			}
			var scopeID = r.ScopeID
			if scope == "overall" {
				scopeID = nil
			}
			if err := PoolAPI.Exec(ctx, `
                INSERT INTO bowling_sequence_features(
                    as_of_date, format_id, scope, scope_id, prev_bowler_id, bowler_id, phase,
                    overs_pairs, balls, runs, wickets, dot_balls
                ) VALUES (
                    $1,$2,$3,$4,$5,$6,$7,
                    $8,$9,$10,$11,$12
                )
                ON CONFLICT (as_of_date, format_id, scope, scope_id0, prev_bowler_id, bowler_id, phase)
                DO UPDATE SET
                    overs_pairs = EXCLUDED.overs_pairs,
                    balls = EXCLUDED.balls,
                    runs = EXCLUDED.runs,
                    wickets = EXCLUDED.wickets,
                    dot_balls = EXCLUDED.dot_balls
            `,
				r.AsOfDate, r.FormatID, scope, scopeID, r.PrevBowlerID, r.BowlerID, r.Phase,
				r.OversPairs, r.Balls, r.Runs, r.Wickets, r.DotBalls,
			); err != nil {
				return err
			}
		}
		return nil
	}

	// Bulk path: COPY rows into a temp staging table, then a single upsert.
	tx, err := PoolAPI.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Create a temp staging table with the precise columns we insert into (scope_id0 is generated in the real table).
	if err := tx.Exec(ctx, `
        CREATE TEMP TABLE bowl_seq_stage AS
        SELECT 
            as_of_date::date,
            format_id::smallint,
            scope::text,
            scope_id::bigint,
            prev_bowler_id::bigint,
            bowler_id::bigint,
            phase::text,
            overs_pairs::int,
            balls::int,
            runs::int,
            wickets::int,
            dot_balls::int
        FROM bowling_sequence_features
        WITH NO DATA;
    `); err != nil {
		return err
	}

	// Build COPY rows with normalized scope fields.
	data := make([][]any, 0, len(rows))
	for i := range rows {
		r := rows[i]
		scope := r.Scope
		if scope == "" {
			scope = "overall"
		}
		var scopeID any = r.ScopeID
		if scope == "overall" {
			scopeID = nil
		}
		data = append(data, []any{
			r.AsOfDate,
			r.FormatID,
			scope,
			scopeID,
			r.PrevBowlerID,
			r.BowlerID,
			r.Phase,
			r.OversPairs,
			r.Balls,
			r.Runs,
			r.Wickets,
			r.DotBalls,
		})
	}

	if _, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"bowl_seq_stage"},
		[]string{
			"as_of_date", "format_id", "scope", "scope_id",
			"prev_bowler_id", "bowler_id", "phase",
			"overs_pairs", "balls", "runs", "wickets", "dot_balls",
		},
		pgx.CopyFromRows(data),
	); err != nil {
		return err
	}

	if err := tx.Exec(ctx, `
        INSERT INTO bowling_sequence_features(
            as_of_date, format_id, scope, scope_id, prev_bowler_id, bowler_id, phase,
            overs_pairs, balls, runs, wickets, dot_balls
        )
        SELECT 
            as_of_date, format_id, scope, scope_id, prev_bowler_id, bowler_id, phase,
            overs_pairs, balls, runs, wickets, dot_balls
        FROM bowl_seq_stage
        ON CONFLICT (as_of_date, format_id, scope, scope_id0, prev_bowler_id, bowler_id, phase)
        DO UPDATE SET
            overs_pairs = EXCLUDED.overs_pairs,
            balls = EXCLUDED.balls,
            runs = EXCLUDED.runs,
            wickets = EXCLUDED.wickets,
            dot_balls = EXCLUDED.dot_balls
    `); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
