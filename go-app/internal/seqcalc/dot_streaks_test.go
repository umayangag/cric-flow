package seqcalc

import (
	"database/sql"
	"testing"
	"time"
)

// ev2 is same as ev helper in reaction tests, duplicated locally for clarity
//
//nolint:unparam // helper accepts many params for clarity; some are constant in tests
func ev2(match int64, inng, ballSeq int, phase string, isLegal bool, striker, bowler int64, runsBat, runsTot int, extrasKind string, outPID int64, asOf time.Time, fmtID int) evRow {
	var sID, bID sql.NullInt64
	if striker != 0 {
		sID = sql.NullInt64{Int64: striker, Valid: true}
	}
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
	return evRow{
		MatchID:     match,
		Innings:     inng,
		BallSeq:     ballSeq,
		Phase:       phase,
		IsLegal:     isLegal,
		StrikerID:   sID,
		BowlerID:    bID,
		RunsBatter:  runsBat,
		RunsTotal:   runsTot,
		ExtrasKind:  ek,
		PlayerOutID: po,
		AsOf:        asOf,
		FormatID:    fmtID,
	}
}

func TestAggregateDotStreaks_KBucketsAndDenominators(t *testing.T) {
	asOf := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	// Build sequence for single striker stream (player 111 vs bowler 211)
	seq := []evRow{
		// b1: legal dot -> initialize k=1
		ev2(1, 1, 1, "powerplay", true, 111, 211, 0, 0, "", 0, asOf, fmtID),
		// b2: legal single -> counted under k=1
		ev2(1, 1, 2, "powerplay", true, 111, 211, 1, 1, "", 0, asOf, fmtID),
		// b3: legal dot -> k=1
		ev2(1, 1, 3, "powerplay", true, 111, 211, 0, 0, "", 0, asOf, fmtID),
		// b4: legal dot -> k=2
		ev2(1, 1, 4, "powerplay", true, 111, 211, 0, 0, "", 0, asOf, fmtID),
		// b5: illegal wide -> recorded under k=2; denominator includes illegal; k stays 2
		ev2(1, 1, 5, "powerplay", false, 111, 211, 0, 1, "wide", 0, asOf, fmtID),
		// b6: legal 4 -> recorded under k=2; k resets to 0
		ev2(1, 1, 6, "powerplay", true, 111, 211, 4, 4, "", 0, asOf, fmtID),
		// b7: legal dot -> k=1
		ev2(1, 1, 7, "powerplay", true, 111, 211, 0, 0, "", 0, asOf, fmtID),
		// b8: legal dot -> k=2
		ev2(1, 1, 8, "powerplay", true, 111, 211, 0, 0, "", 0, asOf, fmtID),
		// b9: legal dot -> k=3
		ev2(1, 1, 9, "powerplay", true, 111, 211, 0, 0, "", 0, asOf, fmtID),
		// b10: legal wicket -> recorded under k=3
		ev2(1, 1, 10, "powerplay", true, 111, 211, 0, 0, "", 111, asOf, fmtID),
	}

	rows := aggregateDotStreaks(seq, true) // batter stream only
	// Build map by k for asserts
	type agg struct{ balls, runs, nb, ns, nw, ne, nd int }
	m := map[int]agg{}
	for _, r := range rows {
		if r.K < 0 || r.K > 6 {
			continue
		}
		m[r.K] = agg{r.Balls, r.RunsNextTotal, r.NextBoundary, r.NextSingle, r.NextWicket, r.NextExtra, r.NextDot}
	}
	// k=0: applies whenever the current kDots==0 before delivery: b1, b3 (after single), b7 (after boundary reset)
	if got := m[0]; got.balls != 3 || got.nd != 3 || got.runs != 0 {
		t.Fatalf("k=0 unexpected: %+v", got)
	}
	// k=1: next balls recorded when kDots==1: b2 (single), b4 (dot), b8 (dot)
	if got := m[1]; got.balls != 3 || got.ns != 1 || got.nd != 2 || got.runs != 1 {
		t.Fatalf("k=1 unexpected: %+v", got)
	}
	// k=2: next balls when kDots==2: b5 (wide, illegal), b6 (boundary), b9 (dot)
	if got := m[2]; got.balls != 3 || got.ne != 1 || got.nb != 1 || got.nd != 1 || got.runs != 5 {
		t.Fatalf("k=2 unexpected: %+v", got)
	}
	// k=3: one next ball (wicket at b10)
	if got := m[3]; got.balls != 1 || got.nw != 1 || got.runs != 0 {
		t.Fatalf("k=3 unexpected: %+v", got)
	}
}
