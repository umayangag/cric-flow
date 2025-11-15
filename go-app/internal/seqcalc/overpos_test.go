package seqcalc

import (
	"database/sql"
	"testing"
	"time"
)

// helper to craft ev rows for over position tests
func evOP(match int64, inng, over, ball int, phase string, isLegal bool, bowler int64, runsBat int, outPID int64, asOf time.Time, fmtID int) evRowOverPos {
	var bID sql.NullInt64
	if bowler != 0 {
		bID = sql.NullInt64{Int64: bowler, Valid: true}
	}
	var po sql.NullInt64
	if outPID != 0 {
		po = sql.NullInt64{Int64: outPID, Valid: true}
	}
	return evRowOverPos{
		MatchID:     match,
		Innings:     inng,
		Over:        over,
		Ball:        ball,
		Phase:       phase,
		IsLegal:     isLegal,
		BowlerID:    bID,
		RunsBatter:  runsBat,
		PlayerOutID: po,
		AsOf:        sql.NullTime{Time: asOf, Valid: true},
		FormatID:    fmtID,
	}
}

func TestAggregateOverPos_Basic(t *testing.T) {
	asOf := time.Date(2024, 8, 10, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(501)
	phase := "powerplay"
	rows := []evRowOverPos{
		// Over 1: ball1 legal, dot; ball6 legal, boundary; include some other balls we will ignore
		evOP(1, 1, 1, 1, phase, true, bow, 0, 0, asOf, fmtID), // position 1
		evOP(1, 1, 1, 2, phase, true, bow, 1, 0, asOf, fmtID),
		evOP(1, 1, 1, 6, phase, true, bow, 4, 0, asOf, fmtID), // position 6 boundary
		// Another over for same bowler
		evOP(1, 1, 2, 1, phase, true, bow, 6, 0, asOf, fmtID),   // position 1 boundary
		evOP(1, 1, 2, 6, phase, true, bow, 0, 123, asOf, fmtID), // position 6 wicket
	}
	agg := aggregateOverPos(rows)
	if len(agg) != 2 {
		t.Fatalf("expected 2 rows (pos 1 and 6), got %d", len(agg))
	}
	var pos1, pos6 *overPosRow
	for i := range agg {
		if agg[i].Position == 1 {
			pos1 = &agg[i]
		}
		if agg[i].Position == 6 {
			pos6 = &agg[i]
		}
	}
	if pos1 == nil || pos6 == nil {
		t.Fatalf("missing positions in aggregation: %+v", agg)
	}
	if pos1.Balls != 2 || pos1.Boundaries != 1 || pos1.Wickets != 0 {
		t.Fatalf("pos1 unexpected counts: %+v", *pos1)
	}
	if pos6.Balls != 2 || pos6.Boundaries != 1 || pos6.Wickets != 1 {
		t.Fatalf("pos6 unexpected counts: %+v", *pos6)
	}
	if pos1.BoundaryRate <= 0 || pos1.BoundaryRate >= 1.0 {
		t.Fatalf("pos1 boundary rate unexpected: %v", pos1.BoundaryRate)
	}
	if pos6.WicketRate <= 0 || pos6.WicketRate >= 1.0 {
		t.Fatalf("pos6 wicket rate unexpected: %v", pos6.WicketRate)
	}
}

func TestAggregateOverPos_IllegalNotCounted(t *testing.T) {
	asOf := time.Date(2024, 8, 11, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(777)
	phase := "middle"
	rows := []evRowOverPos{
		// Illegal at ball 1 should not count
		evOP(2, 1, 8, 1, phase, false, bow, 4, 0, asOf, fmtID),
		// Legal at ball 1 counts
		evOP(2, 1, 8, 1, phase, true, bow, 0, 0, asOf, fmtID),
		// Illegal at ball 6 should not count
		evOP(2, 1, 8, 6, phase, false, bow, 6, 0, asOf, fmtID),
		// Legal at ball 6 counts (wicket)
		evOP(2, 1, 8, 6, phase, true, bow, 0, 909, asOf, fmtID),
	}
	agg := aggregateOverPos(rows)
	if len(agg) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(agg))
	}
	var p1, p6 *overPosRow
	for i := range agg {
		if agg[i].Position == 1 {
			p1 = &agg[i]
		}
		if agg[i].Position == 6 {
			p6 = &agg[i]
		}
	}
	if p1 == nil || p6 == nil {
		t.Fatalf("missing positions in aggregation: %+v", agg)
	}
	if p1.Balls != 1 || p1.Boundaries != 0 || p1.Wickets != 0 {
		t.Fatalf("pos1 counts wrong: %+v", *p1)
	}
	if p6.Balls != 1 || p6.Boundaries != 0 || p6.Wickets != 1 {
		t.Fatalf("pos6 counts wrong: %+v", *p6)
	}
}

func TestAggregateOverPos_PhaseSeparation(t *testing.T) {
	asOf := time.Date(2024, 8, 12, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(888)
	rows := []evRowOverPos{
		evOP(3, 1, 5, 1, "powerplay", true, bow, 0, 0, asOf, fmtID),
		evOP(3, 1, 5, 6, "powerplay", true, bow, 6, 0, asOf, fmtID),
		evOP(3, 1, 17, 1, "death", true, bow, 4, 0, asOf, fmtID),
	}
	agg := aggregateOverPos(rows)
	if len(agg) != 3 { // 2 for powerplay (pos 1 and 6), 1 for death (pos 1)
		t.Fatalf("unexpected rows: %d", len(agg))
	}
	// ensure separate phases present
	var pp1, pp6, d1 bool
	for i := range agg {
		if agg[i].Phase == "powerplay" && agg[i].Position == 1 {
			pp1 = true
		}
		if agg[i].Phase == "powerplay" && agg[i].Position == 6 {
			pp6 = true
		}
		if agg[i].Phase == "death" && agg[i].Position == 1 {
			d1 = true
		}
	}
	if !(pp1 && pp6 && d1) {
		t.Fatalf("phase separation missing: %+v", agg)
	}
}

// 1.13.1 — Multi-format mapping tests for overpos
func TestAggregateOverPos_FormatMapping(t *testing.T) {
	asOf := time.Date(2024, 9, 13, 0, 0, 0, 0, time.UTC)
	bow := int64(9090)
	cases := []struct {
		name  string
		fmtID int
	}{
		{"ODI", 2},
		{"TEST", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := []evRowOverPos{
				// one legal ball at position 1 for simplicity
				evOP(7, 1, 3, 1, "middle", true, bow, 0, 0, asOf, tc.fmtID),
			}
			agg := aggregateOverPos(rows)
			if len(agg) == 0 {
				t.Fatalf("expected rows for format %d", tc.fmtID)
			}
			found := false
			for i := range agg {
				if agg[i].FormatID == tc.fmtID && agg[i].PlayerID == bow {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("did not find aggregated row for fmt %d", tc.fmtID)
			}
		})
	}
}
