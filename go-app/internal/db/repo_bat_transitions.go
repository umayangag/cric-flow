package db

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// BatTransitionRow mirrors columns for insertion into batting_transition_features.
// scope is currently fixed to 'overall' and scope_id=0 as per partial index usage.
type BatTransitionRow struct {
	AsOfDate     string // YYYY-MM-DD
	FormatID     int
	Scope        string // usually 'overall'
	ScopeID      int64  // usually 0
	PrevBatterID int64
	BatterID     int64
	Phase        string
	Balls        int
	Runs         int
	Dismissals   int
	Fours        int
	Sixes        int
}

// UpsertBattingTransitions performs idempotent upserts using the table primary key.
func UpsertBattingTransitions(ctx context.Context, rows []BatTransitionRow) error {
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	if len(rows) == 0 {
		slog.Warn("[UpsertBattingTransitions] no rows to upsert")
		return nil
	}

	// For small batches the COPY overhead can outweigh benefits.
	const smallBatchThreshold = 8
	if len(rows) <= smallBatchThreshold {
		for i := range rows {
			r := rows[i]
			if r.Scope == "" {
				r.Scope = "overall"
			}
			if r.Scope == "overall" && r.ScopeID != 0 {
				r.ScopeID = 0
			}
			if err := PoolAPI.Exec(ctx, `
                INSERT INTO batting_transition_features(
                    as_of_date, format_id, scope, scope_id,
                    prev_batter_id, batter_id, phase,
                    balls, runs, dismissals, fours, sixes
                ) VALUES (
                    $1,$2,$3,$4,
                    $5,$6,$7,
                    $8,$9,$10,$11,$12
                )
                ON CONFLICT (as_of_date, format_id, scope, scope_id, prev_batter_id, batter_id, phase)
                DO UPDATE SET
                    balls = EXCLUDED.balls,
                    runs = EXCLUDED.runs,
                    dismissals = EXCLUDED.dismissals,
                    fours = EXCLUDED.fours,
                    sixes = EXCLUDED.sixes
            `, r.AsOfDate, r.FormatID, r.Scope, r.ScopeID,
				r.PrevBatterID, r.BatterID, r.Phase,
				r.Balls, r.Runs, r.Dismissals, r.Fours, r.Sixes); err != nil {
				return err
			}
		}
		return nil
	}

	// Bulk path using COPY into a temp staging table followed by a single upsert.
	tx, err := PoolAPI.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.Exec(ctx, `DROP TABLE IF EXISTS bat_trans_stage`); err != nil {
		return err
	}
	// Create staging table with column types matching target insert columns.
	if err := tx.Exec(ctx, `
        CREATE TEMP TABLE bat_trans_stage AS
        SELECT 
            as_of_date::date,
            format_id::smallint,
            scope::text,
            scope_id::bigint,
            prev_batter_id::bigint,
            batter_id::bigint,
            phase::text,
            balls::int,
            runs::int,
            dismissals::int,
            fours::int,
            sixes::int
        FROM batting_transition_features
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
		scopeID := r.ScopeID
		if scope == "overall" {
			scopeID = 0
		}
		data = append(data, []any{
			r.AsOfDate,
			r.FormatID,
			scope,
			scopeID,
			r.PrevBatterID,
			r.BatterID,
			r.Phase,
			r.Balls,
			r.Runs,
			r.Dismissals,
			r.Fours,
			r.Sixes,
		})
	}

	if _, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"bat_trans_stage"},
		[]string{
			"as_of_date", "format_id", "scope", "scope_id",
			"prev_batter_id", "batter_id", "phase",
			"balls", "runs", "dismissals", "fours", "sixes",
		},
		pgx.CopyFromRows(data),
	); err != nil {
		return err
	}

	if err := tx.Exec(ctx, `
        INSERT INTO batting_transition_features(
            as_of_date, format_id, scope, scope_id,
            prev_batter_id, batter_id, phase,
            balls, runs, dismissals, fours, sixes
        )
        SELECT 
            as_of_date, format_id, scope, scope_id,
            prev_batter_id, batter_id, phase,
            balls, runs, dismissals, fours, sixes
        FROM bat_trans_stage
        ON CONFLICT (as_of_date, format_id, scope, scope_id, prev_batter_id, batter_id, phase)
        DO UPDATE SET
            balls = EXCLUDED.balls,
            runs = EXCLUDED.runs,
            dismissals = EXCLUDED.dismissals,
            fours = EXCLUDED.fours,
            sixes = EXCLUDED.sixes
    `); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
