package db

import (
	"context"
	"errors"
)

// EventReactionRow mirrors columns for insertion into event_reaction_features.
// scope is fixed to 'overall' and scope_id=0 for latest-as-of partial index.
type EventReactionRow struct {
	AsOfDate           string // YYYY-MM-DD
	FormatID           int
	Scope              string // 'overall'
	ScopeID            int64  // 0
	PlayerID           int64
	Role               string // 'bat' | 'bowl'
	PrevEvent          string // dot,1,2,3,4,6,wide,no_ball,wicket,bye,leg_bye
	Phase              string
	Balls              int
	Runs               int
	Dismissals         int
	Boundaries         int
	Wickets            int
	DotBalls           int
	BoundariesConceded int
}

// UpsertEventReactions performs idempotent upserts using the table primary key.
func UpsertEventReactions(ctx context.Context, rows []EventReactionRow) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	for i := range rows {
		r := rows[i]
		if r.Scope == "" {
			r.Scope = "overall"
		}
		_, err := Pool.Exec(ctx, `
			INSERT INTO event_reaction_features(
				as_of_date, format_id, scope, scope_id,
				player_id, role, prev_event, phase,
				balls, runs, dismissals, boundaries, wickets, dot_balls, boundaries_conceded
			) VALUES (
				$1,$2,$3,$4,
				$5,$6,$7,$8,
				$9,$10,$11,$12,$13,$14,$15
			)
			ON CONFLICT (as_of_date, format_id, scope, scope_id, player_id, role, prev_event, phase)
			DO UPDATE SET
				balls = EXCLUDED.balls,
				runs = EXCLUDED.runs,
				dismissals = EXCLUDED.dismissals,
				boundaries = EXCLUDED.boundaries,
				wickets = EXCLUDED.wickets,
				dot_balls = EXCLUDED.dot_balls,
				boundaries_conceded = EXCLUDED.boundaries_conceded
		`, r.AsOfDate, r.FormatID, r.Scope, r.ScopeID,
			r.PlayerID, r.Role, r.PrevEvent, r.Phase,
			r.Balls, r.Runs, r.Dismissals, r.Boundaries, r.Wickets, r.DotBalls, r.BoundariesConceded)
		if err != nil {
			return err
		}
	}
	return nil
}

// DotStreakRow mirrors columns for insertion into dot_streak_features.
type DotStreakRow struct {
	AsOfDate      string // YYYY-MM-DD
	FormatID      int
	Scope         string // 'overall'
	ScopeID       int64  // 0
	PlayerID      int64
	Role          string // 'bat' | 'bowl'
	K             int    // 0..6
	Phase         string
	Balls         int
	RunsNextTotal int
	NextBoundary  int
	NextSingle    int
	NextWicket    int
	NextExtra     int
	NextDot       int
}

// UpsertDotStreaks performs idempotent upserts using the table primary key.
func UpsertDotStreaks(ctx context.Context, rows []DotStreakRow) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	for i := range rows {
		r := rows[i]
		if r.Scope == "" {
			r.Scope = "overall"
		}
		_, err := Pool.Exec(ctx, `
			INSERT INTO dot_streak_features(
				as_of_date, format_id, scope, scope_id,
				player_id, role, k, phase,
				balls, runs_next_total, next_boundary, next_single, next_wicket, next_extra, next_dot
			) VALUES (
				$1,$2,$3,$4,
				$5,$6,$7,$8,
				$9,$10,$11,$12,$13,$14,$15
			)
			ON CONFLICT (as_of_date, format_id, scope, scope_id, player_id, role, k, phase)
			DO UPDATE SET
				balls = EXCLUDED.balls,
				runs_next_total = EXCLUDED.runs_next_total,
				next_boundary = EXCLUDED.next_boundary,
				next_single = EXCLUDED.next_single,
				next_wicket = EXCLUDED.next_wicket,
				next_extra = EXCLUDED.next_extra,
				next_dot = EXCLUDED.next_dot
		`, r.AsOfDate, r.FormatID, r.Scope, r.ScopeID,
			r.PlayerID, r.Role, r.K, r.Phase,
			r.Balls, r.RunsNextTotal, r.NextBoundary, r.NextSingle, r.NextWicket, r.NextExtra, r.NextDot)
		if err != nil {
			return err
		}
	}
	return nil
}
