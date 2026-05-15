package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpsertBowlingSpells_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	// Run migrations using an absolute path derived from this test package
	require.NoError(t, RunMigrations(ctx, migrationsDir()))

	// Clean table
	require.NoError(t, Exec(ctx, "TRUNCATE bowling_spell_features"))

	// Prepare an initial batch (> smallBatchThreshold to exercise COPY path)
	base := []BowlingSpellRow{
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             101,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       5,
			FirstOversWickets:    0,
			FirstOversDots:       2,
			FirstOversBoundaries: 1,
			LaterOversBalls:      6,
			LaterOversRuns:       4,
			LaterOversWickets:    1,
			LaterOversDots:       3,
			LaterOversBoundaries: 1,
			FirstOverEcon:        5.0,
			LaterOverEcon:        4.0,
		},
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             102,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       4,
			FirstOversWickets:    1,
			FirstOversDots:       3,
			FirstOversBoundaries: 0,
			LaterOversBalls:      6,
			LaterOversRuns:       6,
			LaterOversWickets:    0,
			LaterOversDots:       2,
			LaterOversBoundaries: 2,
			FirstOverEcon:        4.0,
			LaterOverEcon:        6.0,
		},
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             103,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       3,
			FirstOversWickets:    0,
			FirstOversDots:       4,
			FirstOversBoundaries: 0,
			LaterOversBalls:      6,
			LaterOversRuns:       7,
			LaterOversWickets:    1,
			LaterOversDots:       1,
			LaterOversBoundaries: 2,
			FirstOverEcon:        3.0,
			LaterOverEcon:        7.0,
		},
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             104,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       2,
			FirstOversWickets:    1,
			FirstOversDots:       4,
			FirstOversBoundaries: 0,
			LaterOversBalls:      6,
			LaterOversRuns:       8,
			LaterOversWickets:    0,
			LaterOversDots:       1,
			LaterOversBoundaries: 2,
			FirstOverEcon:        2.0,
			LaterOverEcon:        8.0,
		},
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             105,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       6,
			FirstOversWickets:    0,
			FirstOversDots:       2,
			FirstOversBoundaries: 2,
			LaterOversBalls:      6,
			LaterOversRuns:       5,
			LaterOversWickets:    1,
			LaterOversDots:       2,
			LaterOversBoundaries: 1,
			FirstOverEcon:        6.0,
			LaterOverEcon:        5.0,
		},
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             106,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       1,
			FirstOversWickets:    0,
			FirstOversDots:       5,
			FirstOversBoundaries: 0,
			LaterOversBalls:      6,
			LaterOversRuns:       9,
			LaterOversWickets:    0,
			LaterOversDots:       0,
			LaterOversBoundaries: 3,
			FirstOverEcon:        1.0,
			LaterOverEcon:        9.0,
		},
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             107,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       0,
			FirstOversWickets:    1,
			FirstOversDots:       6,
			FirstOversBoundaries: 0,
			LaterOversBalls:      6,
			LaterOversRuns:       6,
			LaterOversWickets:    2,
			LaterOversDots:       2,
			LaterOversBoundaries: 2,
			FirstOverEcon:        0.0,
			LaterOverEcon:        6.0,
		},
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             108,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       2,
			FirstOversWickets:    0,
			FirstOversDots:       4,
			FirstOversBoundaries: 0,
			LaterOversBalls:      6,
			LaterOversRuns:       3,
			LaterOversWickets:    1,
			LaterOversDots:       3,
			LaterOversBoundaries: 1,
			FirstOverEcon:        2.0,
			LaterOverEcon:        3.0,
		},
		{
			AsOfDate:             "2024-01-01",
			FormatID:             1,
			Scope:                "overall",
			ScopeID:              nil,
			PlayerID:             109,
			Phase:                "pp",
			Spells:               1,
			SpellOvers:           2,
			FirstOversBalls:      6,
			FirstOversRuns:       3,
			FirstOversWickets:    0,
			FirstOversDots:       3,
			FirstOversBoundaries: 1,
			LaterOversBalls:      6,
			LaterOversRuns:       4,
			LaterOversWickets:    0,
			LaterOversDots:       2,
			LaterOversBoundaries: 1,
			FirstOverEcon:        3.0,
			LaterOverEcon:        4.0,
		},
	}

	require.NoError(t, UpsertBowlingSpells(ctx, base))

	// Verify count
	var cnt int
	require.NoError(t, QueryRow(ctx, `SELECT count(*) FROM bowling_spell_features`).Scan(&cnt))
	require.Equal(t, len(base), cnt)

	// Upsert a conflicting row with new values to test ON CONFLICT DO UPDATE
	upd := BowlingSpellRow{
		AsOfDate: "2024-01-01", FormatID: 1, Scope: "overall", ScopeID: nil, PlayerID: 101, Phase: "pp",
		Spells: 2, SpellOvers: 4,
		FirstOversBalls: 12, FirstOversRuns: 9, FirstOversWickets: 1, FirstOversDots: 5, FirstOversBoundaries: 2,
		LaterOversBalls: 12, LaterOversRuns: 11, LaterOversWickets: 2, LaterOversDots: 5, LaterOversBoundaries: 3,
		FirstOverEcon: 4.5, LaterOverEcon: 5.5,
	}
	require.NoError(t, UpsertBowlingSpells(ctx, []BowlingSpellRow{upd}))

	// Verify row count unchanged
	require.NoError(t, QueryRow(ctx, `SELECT count(*) FROM bowling_spell_features`).Scan(&cnt))
	require.Equal(t, len(base), cnt)

	// Verify the updated values
	var spells, spellOvers, fBalls, fRuns, fWkts, fDots, fBounds int
	var lBalls, lRuns, lWkts, lDots, lBounds int
	var fEcon, lEcon float64
	require.NoError(t, QueryRow(ctx, `
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
	))
	require.Equal(t, upd.Spells, spells)
	require.Equal(t, upd.SpellOvers, spellOvers)
	require.Equal(t, upd.FirstOversBalls, fBalls)
	require.Equal(t, upd.FirstOversRuns, fRuns)
	require.Equal(t, upd.FirstOversWickets, fWkts)
	require.Equal(t, upd.FirstOversDots, fDots)
	require.Equal(t, upd.FirstOversBoundaries, fBounds)
	require.Equal(t, upd.LaterOversBalls, lBalls)
	require.Equal(t, upd.LaterOversRuns, lRuns)
	require.Equal(t, upd.LaterOversWickets, lWkts)
	require.Equal(t, upd.LaterOversDots, lDots)
	require.Equal(t, upd.LaterOversBoundaries, lBounds)
	require.Equal(t, upd.FirstOverEcon, fEcon)
	require.Equal(t, upd.LaterOverEcon, lEcon)
}
