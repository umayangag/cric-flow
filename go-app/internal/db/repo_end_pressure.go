package db

import (
	"context"
	"errors"
)

// OverEndPressureRow mirrors columns in over_end_pressure_features.
// Scope is 'overall' with NULL scope_id per current plan.
type OverEndPressureRow struct {
	AsOfDate     string // YYYY-MM-DD
	FormatID     int
	Scope        string
	ScopeID      *int64
	PlayerID     int64
	Phase        string
	Position     int // 5 or 6
	Balls        int
	Boundaries   int
	Wickets      int
	BoundaryRate float64
	WicketRate   float64
}

// UpsertOverEndPressure performs idempotent upserts for over_end_pressure_features.
func UpsertOverEndPressure(ctx context.Context, rows []OverEndPressureRow) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	for i := range rows {
		r := rows[i]
		if r.Scope == "" {
			r.Scope = "overall"
		}
		_, err := Pool.Exec(
			ctx, `
			INSERT INTO over_end_pressure_features(
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
