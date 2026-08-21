package seqcalc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// helper to craft evRowSpell quickly
//
//nolint:unparam // helper accepts many params for clarity; some are constant in tests
func evS(
	match int64,
	inng, over, seq int,
	phase string,
	isLegal bool,
	bowler int64,
	runsBat, runsTot int,
	extrasKind string,
	outPID int64,
	asOf time.Time,
	fmtID int,
) evRowSpell {
	var bID sql.NullInt64
	if bowler != 0 {
		bID = sql.NullInt64{Int64: bowler, Valid: true}
	}
	var ek sql.NullString
	if extrasKind != "" {
		ek = sql.NullString{String: extrasKind, Valid: true}
	}
	var po sql.NullInt64
	if outPID != 0 {
		po = sql.NullInt64{Int64: outPID, Valid: true}
	}
	return evRowSpell{
		MatchID:     match,
		Innings:     inng,
		Over:        over,
		BallSeq:     seq,
		Phase:       phase,
		IsLegal:     isLegal,
		BowlerID:    bID,
		RunsBatter:  runsBat,
		RunsTotal:   runsTot,
		ExtrasKind:  ek,
		PlayerOutID: po,
		AsOf:        asOf,
		FormatID:    fmtID,
	}
}

// TestAggregateSpells_SingleTwoOverSpell follows AAA with require assertions.
func TestAggregateSpells_SingleTwoOverSpell(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(201)
	phase := "middle"
	// Two consecutive overs: O10 and O11 same bowler
	// Over 10: legal balls: 6, runs: 6, wickets: 1, dots: 2, boundaries: 1 (four)
	// Over 11: legal balls: 6, runs: 3, wickets: 0, dots: 4, boundaries: 0
	seq := []evRowSpell{
		// Over 10
		evS(1, 1, 10, 1, phase, true, bow, 0, 0, "", 0, asOf, fmtID),      // dot
		evS(1, 1, 10, 2, phase, true, bow, 4, 4, "", 0, asOf, fmtID),      // boundary four
		evS(1, 1, 10, 3, phase, false, bow, 0, 1, "wide", 0, asOf, fmtID), // illegal wide (runs counted)
		evS(1, 1, 10, 4, phase, true, bow, 1, 1, "", 0, asOf, fmtID),      // single
		evS(1, 1, 10, 5, phase, true, bow, 0, 0, "", 123, asOf, fmtID),    // wicket dot
		evS(1, 1, 10, 6, phase, true, bow, 0, 0, "", 0, asOf, fmtID),      // dot
		// Over 11
		evS(1, 1, 11, 7, phase, true, bow, 0, 0, "", 0, asOf, fmtID),  // dot
		evS(1, 1, 11, 8, phase, true, bow, 1, 1, "", 0, asOf, fmtID),  // single
		evS(1, 1, 11, 9, phase, true, bow, 0, 0, "", 0, asOf, fmtID),  // dot
		evS(1, 1, 11, 10, phase, true, bow, 1, 1, "", 0, asOf, fmtID), // single
		evS(1, 1, 11, 11, phase, true, bow, 0, 0, "", 0, asOf, fmtID), // dot
		evS(1, 1, 11, 12, phase, true, bow, 0, 0, "", 0, asOf, fmtID), // dot
	}
	// Act
	rows := aggregateSpells(seq)

	// Assert
	require.Equal(t, 1, len(rows))
	r := rows[0]
	require.Equal(t, bow, r.PlayerID)
	require.Equal(t, fmtID, r.FormatID)
	require.Equal(t, phase, r.Phase)
	require.Equal(t, 1, r.Spells)
	require.Equal(t, 2, r.SpellOvers)
	// First over accumulators (O10): 5 legal (one wide is illegal) => balls=5
	require.Equal(t, 5, r.FirstOversBalls)
	require.Equal(t, 6, r.FirstOversRuns)
	require.Equal(t, 1, r.FirstOversWickets)
	require.Equal(t, 3, r.FirstOversDots) // dot, wicket-dot, dot
	require.Equal(t, 1, r.FirstOversBoundaries)
	// Later over accumulators (O11)
	require.Equal(t, 6, r.LaterOversBalls)
	require.Equal(t, 2, r.LaterOversRuns)
	require.Equal(t, 0, r.LaterOversWickets)
	require.Equal(t, 4, r.LaterOversDots)
	require.Equal(t, 0, r.LaterOversBoundaries)
	// Economy rates
	require.Greater(t, r.FirstOverEcon, 0.0)
	require.Greater(t, r.LaterOverEcon, 0.0)
}

func TestAggregateSpells_BreakCreatesNewSpell(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(302)
	phase := "death"
	seq := []evRowSpell{
		// Over 18
		evS(2, 1, 18, 1, phase, true, bow, 0, 0, "", 0, asOf, fmtID),
		evS(2, 1, 18, 2, phase, true, bow, 1, 1, "", 0, asOf, fmtID),
		evS(2, 1, 18, 3, phase, true, bow, 0, 0, "", 0, asOf, fmtID),
		evS(2, 1, 18, 4, phase, true, bow, 6, 6, "", 0, asOf, fmtID),
		evS(2, 1, 18, 5, phase, true, bow, 0, 0, "", 0, asOf, fmtID),
		evS(2, 1, 18, 6, phase, true, bow, 0, 0, "", 0, asOf, fmtID),
		// Skip over 19 (another bowler bowls), bowler returns for over 20 => new spell
		// Over 20
		evS(2, 1, 20, 7, phase, true, bow, 0, 0, "", 0, asOf, fmtID),
		evS(2, 1, 20, 8, phase, true, bow, 0, 0, "", 0, asOf, fmtID),
		evS(2, 1, 20, 9, phase, true, bow, 1, 1, "", 0, asOf, fmtID),
		evS(2, 1, 20, 10, phase, true, bow, 0, 0, "", 0, asOf, fmtID),
		evS(2, 1, 20, 11, phase, true, bow, 4, 4, "", 0, asOf, fmtID),
		evS(2, 1, 20, 12, phase, true, bow, 0, 0, "", 0, asOf, fmtID),
	}
	rows := aggregateSpells(seq)
	require.Equal(t, 1, len(rows))
	r := rows[0]
	require.Equal(t, 2, r.Spells, "two separate spells expected")
	require.Equal(t, 2, r.SpellOvers)
	require.Greater(t, r.FirstOversBalls, 0)
	require.GreaterOrEqual(t, r.LaterOversBalls, 0)
}

func TestAggregateSpells_FormatMapping(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 9, 12, 0, 0, 0, 0, time.UTC)
	bow := int64(707)
	phase := "middle"
	testCases := []struct {
		name  string
		fmtID int
	}{
		{"ODI", 2},
		{"TEST", 1},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			seq := []evRowSpell{
				// One over only so it's the first-over part of a spell
				evS(5, 1, 10, 1, phase, true, bow, 0, 0, "", 0, asOf, tc.fmtID),
				evS(5, 1, 10, 2, phase, true, bow, 1, 1, "", 0, asOf, tc.fmtID),
				evS(5, 1, 10, 3, phase, true, bow, 0, 0, "", 0, asOf, tc.fmtID),
				evS(5, 1, 10, 4, phase, true, bow, 0, 0, "", 0, asOf, tc.fmtID),
				evS(5, 1, 10, 5, phase, true, bow, 0, 0, "", 0, asOf, tc.fmtID),
				evS(5, 1, 10, 6, phase, true, bow, 0, 0, "", 0, asOf, tc.fmtID),
			}
			rows := aggregateSpells(seq)
			require.NotEmpty(t, rows)
			r := rows[0]
			require.Equal(t, tc.fmtID, r.FormatID)
			require.Equal(t, bow, r.PlayerID)
		})
	}
}
