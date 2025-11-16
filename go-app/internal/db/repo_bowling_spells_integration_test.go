package db

import (
	"context"
	"testing"
)

func TestUpsertBowlingSpells_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Run migrations using an absolute path derived from this test package
	if err := RunMigrations(ctx, migrationsDir()); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	// Clean table
	if err := Exec(ctx, "TRUNCATE bowling_spell_features"); err != nil {
		t.Fatalf("truncate failed: %v", err)
	}

	// Prepare an initial batch (> smallBatchThreshold to exercise COPY path)
	base := []BowlingSpellRow{
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 101, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 5, FirstOversWickets: 0, FirstOversDots: 2, FirstOversBoundaries: 1, LaterOversBalls: 6, LaterOversRuns: 4, LaterOversWickets: 1, LaterOversDots: 3, LaterOversBoundaries: 1, FirstOverEcon: 5.0, LaterOverEcon: 4.0},
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 102, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 4, FirstOversWickets: 1, FirstOversDots: 3, FirstOversBoundaries: 0, LaterOversBalls: 6, LaterOversRuns: 6, LaterOversWickets: 0, LaterOversDots: 2, LaterOversBoundaries: 2, FirstOverEcon: 4.0, LaterOverEcon: 6.0},
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 103, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 3, FirstOversWickets: 0, FirstOversDots: 4, FirstOversBoundaries: 0, LaterOversBalls: 6, LaterOversRuns: 7, LaterOversWickets: 1, LaterOversDots: 1, LaterOversBoundaries: 2, FirstOverEcon: 3.0, LaterOverEcon: 7.0},
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 104, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 2, FirstOversWickets: 1, FirstOversDots: 4, FirstOversBoundaries: 0, LaterOversBalls: 6, LaterOversRuns: 8, LaterOversWickets: 0, LaterOversDots: 1, LaterOversBoundaries: 2, FirstOverEcon: 2.0, LaterOverEcon: 8.0},
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 105, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 6, FirstOversWickets: 0, FirstOversDots: 2, FirstOversBoundaries: 2, LaterOversBalls: 6, LaterOversRuns: 5, LaterOversWickets: 1, LaterOversDots: 2, LaterOversBoundaries: 1, FirstOverEcon: 6.0, LaterOverEcon: 5.0},
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 106, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 1, FirstOversWickets: 0, FirstOversDots: 5, FirstOversBoundaries: 0, LaterOversBalls: 6, LaterOversRuns: 9, LaterOversWickets: 0, LaterOversDots: 0, LaterOversBoundaries: 3, FirstOverEcon: 1.0, LaterOverEcon: 9.0},
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 107, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 0, FirstOversWickets: 1, FirstOversDots: 6, FirstOversBoundaries: 0, LaterOversBalls: 6, LaterOversRuns: 6, LaterOversWickets: 2, LaterOversDots: 2, LaterOversBoundaries: 2, FirstOverEcon: 0.0, LaterOverEcon: 6.0},
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 108, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 2, FirstOversWickets: 0, FirstOversDots: 4, FirstOversBoundaries: 0, LaterOversBalls: 6, LaterOversRuns: 3, LaterOversWickets: 1, LaterOversDots: 3, LaterOversBoundaries: 1, FirstOverEcon: 2.0, LaterOverEcon: 3.0},
		{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 109, Phase: "pp", Spells: 1, SpellOvers: 2, FirstOversBalls: 6, FirstOversRuns: 3, FirstOversWickets: 0, FirstOversDots: 3, FirstOversBoundaries: 1, LaterOversBalls: 6, LaterOversRuns: 4, LaterOversWickets: 0, LaterOversDots: 2, LaterOversBoundaries: 1, FirstOverEcon: 3.0, LaterOverEcon: 4.0},
	}

	if err := UpsertBowlingSpells(ctx, base); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	// Verify count
	var cnt int
	if err := QueryRow(ctx, `SELECT count(*) FROM bowling_spell_features`).Scan(&cnt); err != nil {
		t.Fatalf("count scan failed: %v", err)
	}
	if cnt != len(base) {
		t.Fatalf("unexpected row count: got %d want %d", cnt, len(base))
	}

	// Upsert a conflicting row with new values to test ON CONFLICT DO UPDATE
	upd := BowlingSpellRow{AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 101, Phase: "pp",
		Spells: 2, SpellOvers: 4,
		FirstOversBalls: 12, FirstOversRuns: 9, FirstOversWickets: 1, FirstOversDots: 5, FirstOversBoundaries: 2,
		LaterOversBalls: 12, LaterOversRuns: 11, LaterOversWickets: 2, LaterOversDots: 5, LaterOversBoundaries: 3,
		FirstOverEcon: 4.5, LaterOverEcon: 5.5,
	}
	if err := UpsertBowlingSpells(ctx, []BowlingSpellRow{upd}); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	// Verify row count unchanged
	if err := QueryRow(ctx, `SELECT count(*) FROM bowling_spell_features`).Scan(&cnt); err != nil {
		t.Fatalf("count rescan failed: %v", err)
	}
	if cnt != len(base) {
		t.Fatalf("row count changed after update: got %d want %d", cnt, len(base))
	}

	// Verify the updated values
	var spells, spellOvers, fBalls, fRuns, fWkts, fDots, fBounds int
	var lBalls, lRuns, lWkts, lDots, lBounds int
	var fEcon, lEcon float64
	if err := QueryRow(ctx, `
        SELECT spells, spell_overs,
               first_overs_balls, first_overs_runs, first_overs_wickets, first_overs_dots, first_overs_boundaries,
               later_overs_balls, later_overs_runs, later_overs_wickets, later_overs_dots, later_overs_boundaries,
               first_over_econ, later_over_econ
        FROM bowling_spell_features
        WHERE as_of_date = $1 AND format_id = $2 AND scope = $3 AND scope_id IS NULL
          AND player_id = $4 AND phase = $5
    `, upd.AsOfDate, upd.FormatID, "overall", upd.PlayerID, upd.Phase).Scan(
		&spells, &spellOvers,
		&fBalls, &fRuns, &fWkts, &fDots, &fBounds,
		&lBalls, &lRuns, &lWkts, &lDots, &lBounds,
		&fEcon, &lEcon,
	); err != nil {
		t.Fatalf("select updated row failed: %v", err)
	}

	if spells != upd.Spells || spellOvers != upd.SpellOvers ||
		fBalls != upd.FirstOversBalls || fRuns != upd.FirstOversRuns || fWkts != upd.FirstOversWickets || fDots != upd.FirstOversDots || fBounds != upd.FirstOversBoundaries ||
		lBalls != upd.LaterOversBalls || lRuns != upd.LaterOversRuns || lWkts != upd.LaterOversWickets || lDots != upd.LaterOversDots || lBounds != upd.LaterOversBoundaries ||
		fEcon != upd.FirstOverEcon || lEcon != upd.LaterOverEcon {
		t.Fatalf("updated values mismatch")
	}
}
