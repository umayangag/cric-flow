package seqcalc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// helper to craft evRowWK rows
func evWK(match int64, inng, ballSeq int, phase string, isLegal bool, bowler int64, runsTot int, extrasKind string, outPID int64, wicketKind string, asOf time.Time, fmtID int) evRowWK {
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
	var wk sql.NullString
	if wicketKind != "" {
		wk = sql.NullString{String: wicketKind, Valid: true}
	}
	return evRowWK{
		MatchID:     match,
		Innings:     inng,
		BallSeq:     ballSeq,
		Phase:       phase,
		IsLegal:     isLegal,
		BowlerID:    bID,
		ExtrasKind:  ek,
		RunsTotal:   runsTot,
		PlayerOutID: po,
		WicketKind:  wk,
		AsOf:        sql.NullTime{Time: asOf, Valid: true},
		FormatID:    fmtID,
	}
}

func TestCanonicalMode(t *testing.T) {
	tcs := []struct {
		in   string
		want string
	}{
		{"Caught and Bowled", "caught"},
		{"runout", "run_out"},
		{"Run Out", "run_out"},
		{"LBW", "lbw"},
		{"hit-wicket", "hit_wicket"},
		{"Stumped", "stumped"},
		{"  Caught  ", "caught"},
	}
	for _, tc := range tcs {
		got := canonicalMode(sql.NullString{String: tc.in, Valid: true})
		if got != tc.want {
			t.Fatalf("canonicalMode(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	// empty/null
	if canonicalMode(sql.NullString{}) != "" {
		t.Fatalf("expected empty for invalid null string")
	}
}

func TestAggregateWicketModes_Basic(t *testing.T) {
	asOf := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bowler := int64(42)
	rows := []evRowWK{
		// legal balls, with one wicket (caught)
		evWK(1, 1, 1, "powerplay", true, bowler, 0, "", 0, "", asOf, fmtID),
		evWK(1, 1, 2, "powerplay", true, bowler, 1, "", 0, "", asOf, fmtID),
		// illegal wide should not increment balls
		evWK(1, 1, 3, "powerplay", false, bowler, 1, "wide", 0, "", asOf, fmtID),
		evWK(1, 1, 4, "powerplay", true, bowler, 0, "", 111, "Caught and Bowled", asOf, fmtID),
	}
	agg := aggregateWicketModes(rows)
	if len(agg) != 1 {
		t.Fatalf("expected 1 agg row, got %d", len(agg))
	}
	r := agg[0]
	if r.PlayerID != bowler || r.FormatID != fmtID || r.Phase != "powerplay" || r.Mode != "caught" {
		t.Fatalf("unexpected key fields: %+v", r)
	}
	if r.Balls != 3 { // only legal deliveries counted
		t.Fatalf("Balls expected 3, got %d", r.Balls)
	}
	if r.Wickets != 1 {
		t.Fatalf("Wickets expected 1, got %d", r.Wickets)
	}
	if r.WicketsPer100 <= 0 || r.WicketsPer100 > 40 {
		t.Fatalf("unexpected rate: %v", r.WicketsPer100)
	}
}

func TestAggregateWicketModes_PhaseSeparation(t *testing.T) {
	asOf := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bowler := int64(77)
	rows := []evRowWK{
		evWK(1, 1, 1, "powerplay", true, bowler, 0, "", 0, "", asOf, fmtID),
		evWK(1, 1, 2, "powerplay", true, bowler, 0, "", 0, "", asOf, fmtID),
		evWK(1, 1, 1, "death", true, bowler, 0, "", 0, "", asOf, fmtID),
		evWK(1, 1, 2, "death", true, bowler, 0, "", 222, "lbw", asOf, fmtID),
	}
	agg := aggregateWicketModes(rows)
	if len(agg) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(agg))
	}
	// collect by phase
	var pp, death *db.WicketModeRow
	for i := range agg {
		if agg[i].Phase == "powerplay" {
			pp = &agg[i]
		}
		if agg[i].Phase == "death" {
			death = &agg[i]
		}
	}
	if pp == nil || death == nil {
		t.Fatalf("missing phases in aggregation: %+v", agg)
	}
	if pp.Balls != 2 || pp.Wickets != 0 {
		t.Fatalf("powerplay unexpected counts: %+v", *pp)
	}
	if death.Balls != 2 || death.Wickets != 1 || death.Mode != "lbw" {
		t.Fatalf("death unexpected counts: %+v", *death)
	}
}
