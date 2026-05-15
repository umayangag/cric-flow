package db

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
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
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	if len(rows) == 0 {
		slog.Warn("[UpsertBowlingSpells] no rows to upsert")
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
			scopeID := r.ScopeID
			if scope == "overall" {
				scopeID = nil
			}
			if err := PoolAPI.Exec(
				ctx, `
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
				r.AsOfDate, r.FormatID, scope, scopeID, r.PlayerID, r.Phase,
				r.Spells, r.SpellOvers,
				r.FirstOversBalls, r.FirstOversRuns, r.FirstOversWickets, r.FirstOversDots, r.FirstOversBoundaries,
				r.LaterOversBalls, r.LaterOversRuns, r.LaterOversWickets, r.LaterOversDots, r.LaterOversBoundaries,
				r.FirstOverEcon, r.LaterOverEcon,
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

	// Drop if exists so this works when the same connection is reused (e.g. concurrent seqcalc workers).
	if err := tx.Exec(ctx, `DROP TABLE IF EXISTS bowling_spell_stage`); err != nil {
		return err
	}
	// Create a temp staging table with the precise columns we insert into (scope_id0 is generated in the real table).
	if err := tx.Exec(ctx, `
        CREATE TEMP TABLE bowling_spell_stage AS
        SELECT 
            as_of_date::date,
            format_id::int,
            scope::text,
            scope_id::int,
            player_id::bigint,
            phase::text,
            spells::int,
            spell_overs::int,
            first_overs_balls::int,
            first_overs_runs::int,
            first_overs_wickets::int,
            first_overs_dots::int,
            first_overs_boundaries::int,
            later_overs_balls::int,
            later_overs_runs::int,
            later_overs_wickets::int,
            later_overs_dots::int,
            later_overs_boundaries::int,
            first_over_econ::double precision,
            later_over_econ::double precision
        FROM bowling_spell_features
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
			r.PlayerID,
			r.Phase,
			r.Spells,
			r.SpellOvers,
			r.FirstOversBalls,
			r.FirstOversRuns,
			r.FirstOversWickets,
			r.FirstOversDots,
			r.FirstOversBoundaries,
			r.LaterOversBalls,
			r.LaterOversRuns,
			r.LaterOversWickets,
			r.LaterOversDots,
			r.LaterOversBoundaries,
			r.FirstOverEcon,
			r.LaterOverEcon,
		})
	}

	if _, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"bowling_spell_stage"},
		[]string{
			"as_of_date", "format_id", "scope", "scope_id",
			"player_id", "phase",
			"spells", "spell_overs",
			"first_overs_balls", "first_overs_runs", "first_overs_wickets", "first_overs_dots", "first_overs_boundaries",
			"later_overs_balls", "later_overs_runs", "later_overs_wickets", "later_overs_dots", "later_overs_boundaries",
			"first_over_econ", "later_over_econ",
		},
		pgx.CopyFromRows(data),
	); err != nil {
		return err
	}

	if err := tx.Exec(ctx, `
        INSERT INTO bowling_spell_features(
            as_of_date, format_id, scope, scope_id, player_id, phase,
            spells, spell_overs,
            first_overs_balls, first_overs_runs, first_overs_wickets, first_overs_dots, first_overs_boundaries,
            later_overs_balls, later_overs_runs, later_overs_wickets, later_overs_dots, later_overs_boundaries,
            first_over_econ, later_over_econ
        )
        SELECT 
            as_of_date, format_id, scope, scope_id, player_id, phase,
            spells, spell_overs,
            first_overs_balls, first_overs_runs, first_overs_wickets, first_overs_dots, first_overs_boundaries,
            later_overs_balls, later_overs_runs, later_overs_wickets, later_overs_dots, later_overs_boundaries,
            first_over_econ, later_over_econ
        FROM bowling_spell_stage
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
    `); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
