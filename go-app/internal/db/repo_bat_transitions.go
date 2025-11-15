package db

import (
	"context"
	"errors"
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
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	for i := range rows {
		r := rows[i]
		if r.Scope == "" {
			r.Scope = "overall"
		}
		// normalize scope_id when using overall scope
		if r.Scope == "overall" && r.ScopeID == 0 {
			// already 0
		}
		_, err := Pool.Exec(ctx, `
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
			r.Balls, r.Runs, r.Dismissals, r.Fours, r.Sixes)
		if err != nil {
			return err
		}
	}
	return nil
}
