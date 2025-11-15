package db

import (
	"context"
	"errors"
)

// BowlingSpellRow mirrors columns for insertion/upsert into bowling_spell_features.
// Scope is currently 'overall' with NULL scope_id per plan.
type BowlingSpellRow struct {
	AsOfDate string // YYYY-MM-DD
	FormatID int
	Scope    string
	ScopeID  *int64
	PlayerID int64
	Phase    string

	Spells               int
	SpellOvers           int
	FirstOversBalls      int
	FirstOversRuns       int
	FirstOversWickets    int
	FirstOversDots       int
	FirstOversBoundaries int
	LaterOversBalls      int
	LaterOversRuns       int
	LaterOversWickets    int
	LaterOversDots       int
	LaterOversBoundaries int
	FirstOverEcon        float64
	LaterOverEcon        float64
}

// UpsertBowlingSpells performs idempotent upserts for bowling_spell_features rows.
func UpsertBowlingSpells(ctx context.Context, rows []BowlingSpellRow) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	for i := range rows {
		r := rows[i]
		if r.Scope == "" {
			r.Scope = "overall"
		}
		_, err := Pool.Exec(ctx, `
			INSERT INTO bowling_spell_features(
				as_of_date, format_id, scope, scope_id, player_id, phase,
				spells, spell_overs,
				first_overs_balls, first_overs_runs, first_overs_wickets, first_overs_dots, first_overs_boundaries,
				later_overs_balls, later_overs_runs, later_overs_wickets, later_overs_dots, later_overs_boundaries,
				first_over_econ, later_over_econ
			) VALUES (
				$1,$2,$3,$4,$5,$6,
				$7,$8,
				$9,$10,$11,$12,$13,
				$14,$15,$16,$17,$18,
				$19,$20
			)
			ON CONFLICT (as_of_date, format_id, scope, scope_id0, player_id, phase)
			DO UPDATE SET
				spells = EXCLUDED.spells,
				spell_overs = EXCLUDED.spell_overs,
				first_overs_balls = EXCLUDED.first_overs_balls,
				first_overs_runs = EXCLUDED.first_overs_runs,
				first_overs_wickets = EXCLUDED.first_overs_wickets,
				first_overs_dots = EXCLUDED.first_overs_dots,
				first_overs_boundaries = EXCLUDED.first_overs_boundaries,
				later_overs_balls = EXCLUDED.later_overs_balls,
				later_overs_runs = EXCLUDED.later_overs_runs,
				later_overs_wickets = EXCLUDED.later_overs_wickets,
				later_overs_dots = EXCLUDED.later_overs_dots,
				later_overs_boundaries = EXCLUDED.later_overs_boundaries,
				first_over_econ = EXCLUDED.first_over_econ,
				later_over_econ = EXCLUDED.later_over_econ
		`,
			r.AsOfDate, r.FormatID, r.Scope, r.ScopeID, r.PlayerID, r.Phase,
			r.Spells, r.SpellOvers,
			r.FirstOversBalls, r.FirstOversRuns, r.FirstOversWickets, r.FirstOversDots, r.FirstOversBoundaries,
			r.LaterOversBalls, r.LaterOversRuns, r.LaterOversWickets, r.LaterOversDots, r.LaterOversBoundaries,
			r.FirstOverEcon, r.LaterOverEcon,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
