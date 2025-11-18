package seqcalc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

// TestAggregateReaction_BatterAndBowlerStreams follows the gold standard: AAA with require assertions.
func TestAggregateReaction_BatterAndBowlerStreams(t *testing.T) {
	t.Parallel()
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

	// Act
	batRows := aggregateReaction(seq, true)
	bowlRows := aggregateReaction(seq, false)

	// Assert: Convert to map for asserts
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
	if got, ok := bat["dot"]; ok { // b2 under prev=dot
		require.Equal(t, 1, got.balls, "bat prev=dot balls")
		require.Equal(t, 1, got.runs, "bat prev=dot runs")
	} else {
		t.Fatalf("missing bat prev=dot aggregate")
	}
	if got := bat["1"]; true { // after single, next was wide (illegal) with 1 run
		require.Equal(t, 1, got.balls, "bat prev=1 balls")
		require.Equal(t, 0, got.bnd, "bat prev=1 boundaries")
		require.Equal(t, 1, got.runs, "bat prev=1 runs")
	}
	if got := bat["wide"]; true { // after wide, next was boundary 4
		require.Equal(t, 1, got.balls, "bat prev=wide balls")
		require.Equal(t, 1, got.bnd, "bat prev=wide boundaries")
		require.Equal(t, 4, got.runs, "bat prev=wide runs")
	}
	// New striker has no prev event in his own stream; no record under prev=wicket for batter stream
	if got, ok := bat["wicket"]; ok {
		require.Equal(t, 0, got.balls, "bat prev=wicket balls")
		require.Equal(t, 0, got.runs, "bat prev=wicket runs")
	}

	// Expectations for bowler stream
	if got := bowl["wide"]; true { // b4 after wide conceded a boundary next
		require.Equal(t, 1, got.balls, "bowl prev=wide balls")
		require.Equal(t, 1, got.bcon, "bowl prev=wide boundaries conceded")
		require.Equal(t, 4, got.runs, "bowl prev=wide runs")
	}
	if got := bowl["4"]; true { // b5 after 4 produced a wicket
		require.Equal(t, 1, got.balls, "bowl prev=4 balls")
		require.Equal(t, 1, got.wkts, "bowl prev=4 wickets")
	}
}
