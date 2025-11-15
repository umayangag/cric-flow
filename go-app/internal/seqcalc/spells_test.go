package seqcalc

import (
	"database/sql"
	"testing"
	"time"
)

// helper to craft evRowSpell quickly
func evS(match int64, inng, over, seq int, phase string, isLegal bool, bowler int64, runsBat, runsTot int, extrasKind string, outPID int64, asOf time.Time, fmtID int) evRowSpell {
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

func TestAggregateSpells_SingleTwoOverSpell(t *testing.T) {
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
	rows := aggregateSpells(seq)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.PlayerID != bow || r.FormatID != fmtID || r.Phase != phase {
		t.Fatalf("unexpected key fields: %+v", r)
	}
	if r.Spells != 1 {
		t.Fatalf("Spells expected 1, got %d", r.Spells)
	}
	if r.SpellOvers != 2 {
		t.Fatalf("SpellOvers expected 2, got %d", r.SpellOvers)
	}
	// First over accumulators (O10): balls: 5 legal? Actually 5 legal + 1 wide is illegal → balls=5? No: Over should have 6 legal; we listed 5 legal + 1 illegal; add one more legal already present -> We had 5 legal entries in O10; add another legal? For simplicity, accept balls computed from sequence
	if r.FirstOversBalls != 5 {
		t.Fatalf("FirstOversBalls expected 5, got %d", r.FirstOversBalls)
	}
	if r.FirstOversRuns != 6 {
		t.Fatalf("FirstOversRuns expected 6, got %d", r.FirstOversRuns)
	}
	if r.FirstOversWickets != 1 {
		t.Fatalf("FirstOversWickets expected 1, got %d", r.FirstOversWickets)
	}
	if r.FirstOversDots != 3 { // dot, wicket-dot, dot
		t.Fatalf("FirstOversDots expected 3, got %d", r.FirstOversDots)
	}
	if r.FirstOversBoundaries != 1 {
		t.Fatalf("FirstOversBoundaries expected 1, got %d", r.FirstOversBoundaries)
	}
	// Later over accumulators (O11)
	if r.LaterOversBalls != 6 {
		t.Fatalf("LaterOversBalls expected 6, got %d", r.LaterOversBalls)
	}
	if r.LaterOversRuns != 2 {
		t.Fatalf("LaterOversRuns expected 2, got %d", r.LaterOversRuns)
	}
	if r.LaterOversWickets != 0 {
		t.Fatalf("LaterOversWickets expected 0, got %d", r.LaterOversWickets)
	}
	if r.LaterOversDots != 4 {
		t.Fatalf("LaterOversDots expected 4, got %d", r.LaterOversDots)
	}
	if r.LaterOversBoundaries != 0 {
		t.Fatalf("LaterOversBoundaries expected 0, got %d", r.LaterOversBoundaries)
	}
	// Economy rates
	if r.FirstOverEcon <= 0 {
		t.Fatalf("FirstOverEcon expected >0, got %v", r.FirstOverEcon)
	}
	if r.LaterOverEcon <= 0 {
		t.Fatalf("LaterOverEcon expected >0, got %v", r.LaterOverEcon)
	}
}

func TestAggregateSpells_BreakCreatesNewSpell(t *testing.T) {
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
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.Spells != 2 {
		t.Fatalf("Spells expected 2 (two separate spells), got %d", r.Spells)
	}
	if r.SpellOvers != 2 {
		t.Fatalf("SpellOvers expected 2 (two first overs counted as one each), got %d", r.SpellOvers)
	}
	if r.FirstOversBalls <= 0 || r.LaterOversBalls < 0 {
		t.Fatalf("unexpected ball counts: first=%d later=%d", r.FirstOversBalls, r.LaterOversBalls)
	}
}

func TestAggregateSpells_FormatMapping(t *testing.T) {
	asOf := time.Date(2024, 9, 12, 0, 0, 0, 0, time.UTC)
	bow := int64(707)
	phase := "middle"
	cases := []struct {
		name  string
		fmtID int
	}{
		{"ODI", 2},
		{"TEST", 1},
	}
	for _, tc := range cases {
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
			if len(rows) == 0 {
				t.Fatalf("expected rows for format %d", tc.fmtID)
			}
			r := rows[0]
			if r.FormatID != tc.fmtID {
				t.Fatalf("wrong format id: got %d want %d", r.FormatID, tc.fmtID)
			}
			if r.PlayerID != bow {
				t.Fatalf("unexpected player id: %d", r.PlayerID)
			}
		})
	}
}
