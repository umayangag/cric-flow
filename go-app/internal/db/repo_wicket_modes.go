package db

import (
	"context"
	"errors"
)

// WicketModeRow mirrors columns for insertion/upsert into wicket_mode_features.
// Scope is 'overall' with NULL scope_id per current plan.
type WicketModeRow struct {
	AsOfDate string // YYYY-MM-DD
	FormatID int
	Scope    string
	ScopeID  *int64
	PlayerID int64
	Phase    string
	Mode     string
	Balls    int
	Wickets  int
	// convenience precomputed rate (per 100 balls for the given mode)
	WicketsPer100 float64
}

// UpsertWicketModes performs idempotent upserts.
func UpsertWicketModes(ctx context.Context, rows []WicketModeRow) error {
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
			INSERT INTO wicket_mode_features(
				as_of_date, format_id, scope, scope_id, player_id, phase, mode,
				balls, wickets, wickets_per_100
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,
				$8,$9,$10
			)
			ON CONFLICT (as_of_date, format_id, scope, scope_id0, player_id, phase, mode)
			DO UPDATE SET
				balls = EXCLUDED.balls,
				wickets = EXCLUDED.wickets,
				wickets_per_100 = EXCLUDED.wickets_per_100
		`,
			r.AsOfDate, r.FormatID, r.Scope, r.ScopeID, r.PlayerID, r.Phase, r.Mode,
			r.Balls, r.Wickets, r.WicketsPer100,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
