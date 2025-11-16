package seqcalc

import (
	"database/sql"
	"testing"
	"time"
)

// helper to quickly build an evRow
//
//nolint:unparam // helper accepts many params for clarity; some are constant in tests
func ev(
	match int64,
	inng, ballSeq int,
	phase string,
	isLegal bool,
	striker, bowler int64,
	runsBat, runsTot int,
	extrasKind string,
	outPID int64,
	asOf time.Time,
	fmtID int,
) evRow {
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

func TestAggregateReaction_BatterAndBowlerStreams(t *testing.T) {
	// Sequence:
	// b1: dot (prev=none)
	// b2: single (counts under prev=dot)
	// b3: wide (illegal) (counts under prev=1) — denominator includes illegal
	// b4: 4 (counts under prev=wide)
	// b5: wicket (striker out) (counts under prev=4)
	// b6: dot by new striker (counts under prev=wicket)
	asOf := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	seq := []evRow{
		ev(1, 1, 1, "powerplay", true, 101, 201, 0, 0, "", 0, asOf, fmtID), // prev=nil
		ev(1, 1, 2, "powerplay", true, 101, 201, 1, 1, "", 0, asOf, fmtID), // prev=dot -> record under prev=dot
		ev(
			1,
			1,
			3,
			"powerplay",
			false,
			101,
			201,
			0,
			1,
			"wide",
			0,
			asOf,
			fmtID,
		), // prev=1 -> record under prev=1 (illegal included)
		ev(1, 1, 4, "powerplay", true, 101, 201, 4, 4, "", 0, asOf, fmtID), // prev=wide -> under prev=wide
		ev(
			1,
			1,
			5,
			"powerplay",
			true,
			101,
			201,
			0,
			0,
			"",
			101,
			asOf,
			fmtID,
		), // prev=4 -> under prev=4 (wicket event priority if it were combined)
		ev(
			1,
			1,
			6,
			"powerplay",
			true,
			103,
			201,
			0,
			0,
			"",
			0,
			asOf,
			fmtID,
		), // new striker after wicket -> under prev=wicket
	}

	batRows := aggregateReaction(seq, true)
	bowlRows := aggregateReaction(seq, false)

	// Convert to map for asserts
	bat := map[string]struct{ balls, runs, bnd, dism int }{}
	for _, r := range batRows {
		key := r.PrevEvent
		bat[key] = struct{ balls, runs, bnd, dism int }{r.Balls, r.Runs, r.Boundaries, r.Dismissals}
	}
	bowl := map[string]struct{ balls, runs, dots, wkts, bcon int }{}
	for _, r := range bowlRows {
		key := r.PrevEvent
		bowl[key] = struct{ balls, runs, dots, wkts, bcon int }{
			r.Balls,
			r.Runs,
			r.DotBalls,
			r.Wickets,
			r.BoundariesConceded,
		}
	}

	// Expectations for batter stream
	if got := bat["dot"]; true { // b2 under prev=dot
		if got.balls != 1 {
			t.Fatalf("bat prev=dot balls unexpected: %+v", got)
		}
		if got.runs != 1 {
			t.Fatalf("bat prev=dot runs unexpected: %+v", got)
		}
	}
	if got := bat["1"]; true { // after single, next was wide (illegal) with 1 run
		if got.balls != 1 {
			t.Fatalf("bat prev=1 balls unexpected: %+v", got)
		}
		if got.bnd != 0 {
			t.Fatalf("bat prev=1 boundaries unexpected: %+v", got)
		}
		if got.runs != 1 {
			t.Fatalf("bat prev=1 runs unexpected: %+v", got)
		}
	}
	if got := bat["wide"]; true { // after wide, next was boundary 4
		if got.balls != 1 {
			t.Fatalf("bat prev=wide balls unexpected: %+v", got)
		}
		if got.bnd != 1 {
			t.Fatalf("bat prev=wide boundaries unexpected: %+v", got)
		}
		if got.runs != 4 {
			t.Fatalf("bat prev=wide runs unexpected: %+v", got)
		}
	}
	// New striker has no prev event in his own stream; no record under prev=wicket for batter stream
	if got, ok := bat["wicket"]; ok && (got.balls != 0 || got.runs != 0) {
		t.Fatalf("bat prev=wicket should be zero, got: %+v", got)
	}

	// Expectations for bowler stream
	if got := bowl["wide"]; true { // b4 after wide conceded a boundary next
		if got.balls != 1 {
			t.Fatalf("bowl prev=wide balls unexpected: %+v", got)
		}
		if got.bcon != 1 {
			t.Fatalf("bowl prev=wide boundaries conceded unexpected: %+v", got)
		}
		if got.runs != 4 {
			t.Fatalf("bowl prev=wide runs unexpected: %+v", got)
		}
	}
	if got := bowl["4"]; true { // b5 after 4 produced a wicket
		if got.balls != 1 {
			t.Fatalf("bowl prev=4 balls unexpected: %+v", got)
		}
		if got.wkts != 1 {
			t.Fatalf("bowl prev=4 wickets unexpected: %+v", got)
		}
	}
}
