package db

import (
	"context"
	"errors"
)

// BallEventRow mirrors columns for insertion into ball_event.
// Optional player references are pointers; nil means unresolved/unknown.
// fielder_ids is omitted in this version for simplicity (left NULL).
type BallEventRow struct {
	MatchID      int64
	Innings      int
	Over         int
	Ball         int
	BallSeq      int
	IsLegal      bool
	Phase        string
	StrikerID    *int64
	NonStrikerID *int64
	BowlerID     *int64
	RunsBatter   int
	RunsExtras   int
	RunsTotal    int
	ExtrasKind   *string
	WicketKind   *string
	PlayerOutID  *int64
}

// InsertBallEvents inserts rows idempotently using the natural primary key
// (match_id, innings, over, ball). On conflicts, it does nothing to remain safe
// for re-runs and backfills.
func InsertBallEvents(ctx context.Context, rows []BallEventRow) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	for i := range rows {
		r := rows[i]
		_, err := Pool.Exec(ctx, `
			INSERT INTO ball_event(
				match_id, innings, over, ball, ball_seq, is_legal, phase,
				striker_id, non_striker_id, bowler_id,
				runs_batter, runs_extras, runs_total,
				extras_kind, wicket_kind, player_out_id
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,
				$8,$9,$10,
				$11,$12,$13,
				$14,$15,$16
			)
			ON CONFLICT (match_id, innings, over, ball) DO NOTHING
		`,
			r.MatchID, r.Innings, r.Over, r.Ball, r.BallSeq, r.IsLegal, r.Phase,
			r.StrikerID, r.NonStrikerID, r.BowlerID,
			r.RunsBatter, r.RunsExtras, r.RunsTotal,
			r.ExtrasKind, r.WicketKind, r.PlayerOutID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
