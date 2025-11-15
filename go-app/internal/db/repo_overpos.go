package db

import (
	"context"
	"errors"
)

// OverPosRow mirrors columns for insertion/upsert into over_boundary_wicket_features.
// Scope is currently 'overall' with NULL scope_id as per plan.
type OverPosRow struct {
	AsOfDate     string // YYYY-MM-DD
	FormatID     int
	Scope        string
	ScopeID      *int64
	PlayerID     int64
	Phase        string
	Position     int // 1 or 6
	Balls        int
	Boundaries   int
	Wickets      int
	BoundaryRate float64
	WicketRate   float64
}

// UpsertOverPos performs idempotent upserts for over_boundary_wicket_features rows.
func UpsertOverPos(ctx context.Context, rows []OverPosRow) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	for i := range rows {
		r := rows[i]
		if r.Scope == "" {
			r.Scope = "overall"
		}
		_, err := Pool.Exec(ctx, `
			INSERT INTO over_boundary_wicket_features(
				as_of_date, format_id, scope, scope_id, player_id, phase, position,
				balls, boundaries, wickets,
				boundary_rate, wicket_rate
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,
				$8,$9,$10,
				$11,$12
			)
			ON CONFLICT (as_of_date, format_id, scope, scope_id0, player_id, phase, position)
			DO UPDATE SET
				balls = EXCLUDED.balls,
				boundaries = EXCLUDED.boundaries,
				wickets = EXCLUDED.wickets,
				boundary_rate = EXCLUDED.boundary_rate,
				wicket_rate = EXCLUDED.wicket_rate
		`,
			r.AsOfDate, r.FormatID, r.Scope, r.ScopeID, r.PlayerID, r.Phase, r.Position,
			r.Balls, r.Boundaries, r.Wickets,
			r.BoundaryRate, r.WicketRate,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
