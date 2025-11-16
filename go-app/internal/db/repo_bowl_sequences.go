package db

import (
	"context"
	"errors"
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
	for i := range rows {
		r := rows[i]
		_, err := Pool.Exec(ctx, `
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
			r.AsOfDate, r.FormatID, r.Scope, r.ScopeID, r.PrevBowlerID, r.BowlerID, r.Phase,
			r.OversPairs, r.Balls, r.Runs, r.Wickets, r.DotBalls,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
