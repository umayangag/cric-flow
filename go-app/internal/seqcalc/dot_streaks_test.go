package seqcalc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ev2 is same as ev helper in reaction tests, duplicated locally for clarity
//
//nolint:unparam // helper accepts many params for clarity; some are constant in tests
func ev2(
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

// TestAggregateDotStreaks_KBucketsAndDenominators follows the gold standard: AAA with require assertions.
func TestAggregateDotStreaks_KBucketsAndDenominators(t *testing.T) {
	t.Parallel()

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

	// Act
	rows := aggregateDotStreaks(seq, true) // batter stream only

	// Assert: Build map by k for asserts
	type agg struct{ balls, runs, nb, ns, nw, ne, nd int }
	m := map[int]agg{}
	for _, r := range rows {
		if r.K < 0 || r.K > 6 {
			continue
		}
		m[r.K] = agg{r.Balls, r.RunsNextTotal, r.NextBoundary, r.NextSingle, r.NextWicket, r.NextExtra, r.NextDot}
	}
	// k=0: applies whenever the current kDots==0 before delivery: b1, b3 (after single), b7 (after boundary reset)
	got := m[0]
	require.Equal(t, 3, got.balls, "k=0 balls")
	require.Equal(t, 3, got.nd, "k=0 next dots")
	require.Equal(t, 0, got.runs, "k=0 runs")
	// k=1: next balls recorded when kDots==1: b2 (single), b4 (dot), b8 (dot)
	g1 := m[1]
	require.Equal(t, 3, g1.balls, "k=1 balls")
	require.Equal(t, 1, g1.ns, "k=1 next singles")
	require.Equal(t, 2, g1.nd, "k=1 next dots")
	require.Equal(t, 1, g1.runs, "k=1 runs")
	// k=2: next balls when kDots==2: b5 (wide, illegal), b6 (boundary), b9 (dot)
	g2 := m[2]
	require.Equal(t, 3, g2.balls, "k=2 balls")
	require.Equal(t, 1, g2.ne, "k=2 next extras")
	require.Equal(t, 1, g2.nb, "k=2 next boundaries")
	require.Equal(t, 1, g2.nd, "k=2 next dots")
	require.Equal(t, 5, g2.runs, "k=2 runs")
	// k=3: one next ball (wicket at b10)
	g3 := m[3]
	require.Equal(t, 1, g3.balls, "k=3 balls")
	require.Equal(t, 1, g3.nw, "k=3 next wickets")
	require.Equal(t, 0, g3.runs, "k=3 runs")
}
