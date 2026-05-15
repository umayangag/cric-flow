package seqcalc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// helper to craft evRowEP rows quickly
//
//nolint:unparam // helper accepts many params for clarity; some are constant in tests
func evEP(
	match int64,
	inng, over, seq int,
	phase string,
	isLegal bool,
	bowler int64,
	runsBat, runsTot int,
	outPID int64,
	asOf time.Time,
	fmtID int,
) evRowEP {
	var bID sql.NullInt64
	if bowler != 0 {
		bID = sql.NullInt64{Int64: bowler, Valid: true}
	}
	var po sql.NullInt64
	if outPID != 0 {
		po = sql.NullInt64{Int64: outPID, Valid: true}
	}
	return evRowEP{
		MatchID:     match,
		Innings:     inng,
		Over:        over,
		BallSeq:     seq,
		Phase:       phase,
		IsLegal:     isLegal,
		BowlerID:    bID,
		RunsBatter:  runsBat,
		RunsTotal:   runsTot,
		PlayerOutID: po,
		AsOf:        asOf,
		FormatID:    fmtID,
	}
}

// TestAggregateEndPressure_Basic follows AAA with require assertions.
func TestAggregateEndPressure_Basic(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(501)
	phase := "death"
	// Build one over with a mix of legal and illegal to ensure correct legal positions 5 and 6.
	// Legal positions (by legal deliveries only): 1,2,3,4,5,6
	// We'll place: pos5 = boundary (4), pos6 = wicket dot
	seq := []evRowEP{
		// Over 20
		evEP(1, 1, 20, 1, phase, true, bow, 1, 1, 0, asOf, fmtID),   // pos1
		evEP(1, 1, 20, 2, phase, true, bow, 0, 0, 0, asOf, fmtID),   // pos2
		evEP(1, 1, 20, 3, phase, false, bow, 0, 1, 0, asOf, fmtID),  // illegal wide (does not advance)
		evEP(1, 1, 20, 4, phase, true, bow, 1, 1, 0, asOf, fmtID),   // pos3
		evEP(1, 1, 20, 5, phase, true, bow, 0, 0, 0, asOf, fmtID),   // pos4
		evEP(1, 1, 20, 6, phase, true, bow, 4, 4, 0, asOf, fmtID),   // pos5 boundary
		evEP(1, 1, 20, 7, phase, true, bow, 0, 0, 999, asOf, fmtID), // pos6 wicket
	}
	// Act
	rows := aggregateEndPressure(seq)
	// Assert
	require.Equal(t, 2, len(rows), "expected positions 5 and 6 only")
	// collect by position
	var p5, p6 *db.OverEndPressureRow
	for i := range rows {
		if rows[i].Position == 5 {
			p5 = &rows[i]
		}
		if rows[i].Position == 6 {
			p6 = &rows[i]
		}
	}
	require.NotNil(t, p5, "missing pos5 row: %+v", rows)
	require.NotNil(t, p6, "missing pos6 row: %+v", rows)
	require.Equal(t, bow, p5.PlayerID)
	require.Equal(t, fmtID, p5.FormatID)
	require.Equal(t, phase, p5.Phase)
	require.Equal(t, 1, p5.Balls)
	require.Equal(t, 1, p5.Boundaries)
	require.Equal(t, 0, p5.Wickets)
	require.Equal(t, 1, p6.Balls)
	require.Equal(t, 0, p6.Boundaries)
	require.Equal(t, 1, p6.Wickets)
	require.Equal(t, 1.0, p5.BoundaryRate)
	require.Equal(t, 0.0, p5.WicketRate)
	require.Equal(t, 0.0, p6.BoundaryRate)
	require.Equal(t, 1.0, p6.WicketRate)
}

func TestAggregateEndPressure_IllegalNotCounted(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 8, 2, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(602)
	phase := "powerplay"
	seq := []evRowEP{
		// Over 3 with many illegals; ensure legal positions still end at the correct deliveries
		evEP(2, 1, 3, 1, phase, true, bow, 0, 0, 0, asOf, fmtID),  // pos1
		evEP(2, 1, 3, 2, phase, false, bow, 0, 1, 0, asOf, fmtID), // illegal wide
		evEP(2, 1, 3, 3, phase, true, bow, 0, 0, 0, asOf, fmtID),  // pos2
		evEP(2, 1, 3, 4, phase, false, bow, 0, 1, 0, asOf, fmtID), // illegal wide
		evEP(2, 1, 3, 5, phase, true, bow, 0, 0, 0, asOf, fmtID),  // pos3
		evEP(2, 1, 3, 6, phase, true, bow, 0, 0, 0, asOf, fmtID),  // pos4
		evEP(2, 1, 3, 7, phase, false, bow, 0, 1, 0, asOf, fmtID), // illegal wide
		evEP(2, 1, 3, 8, phase, true, bow, 4, 4, 0, asOf, fmtID),  // pos5 boundary
		evEP(2, 1, 3, 9, phase, false, bow, 0, 1, 0, asOf, fmtID), // illegal wide
		evEP(2, 1, 3, 10, phase, true, bow, 0, 0, 0, asOf, fmtID), // pos6 not boundary
	}
	// Act
	rows := aggregateEndPressure(seq)
	// Assert
	require.Equal(t, 2, len(rows))
	var p5, p6 *db.OverEndPressureRow
	for i := range rows {
		if rows[i].Position == 5 {
			p5 = &rows[i]
		}
		if rows[i].Position == 6 {
			p6 = &rows[i]
		}
	}
	require.NotNil(t, p5)
	require.NotNil(t, p6)
	require.Equal(t, 1, p5.Balls)
	require.Equal(t, 1, p5.Boundaries)
	require.Equal(t, 1, p6.Balls)
	require.Equal(t, 0, p6.Boundaries)
}

func TestAggregateEndPressure_PhaseSeparation(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 8, 3, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(703)
	seq := []evRowEP{
		// powerplay over
		evEP(3, 1, 4, 1, "powerplay", true, bow, 0, 0, 0, asOf, fmtID), // pos1
		evEP(3, 1, 4, 2, "powerplay", true, bow, 0, 0, 0, asOf, fmtID), // pos2
		evEP(3, 1, 4, 3, "powerplay", true, bow, 0, 0, 0, asOf, fmtID), // pos3
		evEP(3, 1, 4, 4, "powerplay", true, bow, 0, 0, 0, asOf, fmtID), // pos4
		evEP(3, 1, 4, 5, "powerplay", true, bow, 0, 0, 0, asOf, fmtID), // pos5
		evEP(3, 1, 4, 6, "powerplay", true, bow, 0, 0, 0, asOf, fmtID), // pos6
		// death over
		evEP(3, 1, 20, 7, "death", true, bow, 0, 0, 0, asOf, fmtID),    // pos1
		evEP(3, 1, 20, 8, "death", true, bow, 0, 0, 0, asOf, fmtID),    // pos2
		evEP(3, 1, 20, 9, "death", true, bow, 0, 0, 0, asOf, fmtID),    // pos3
		evEP(3, 1, 20, 10, "death", true, bow, 0, 0, 0, asOf, fmtID),   // pos4
		evEP(3, 1, 20, 11, "death", true, bow, 6, 6, 0, asOf, fmtID),   // pos5 boundary
		evEP(3, 1, 20, 12, "death", true, bow, 0, 0, 404, asOf, fmtID), // pos6 wicket
	}
	// Act
	rows := aggregateEndPressure(seq)
	// Assert
	require.Equal(t, 4, len(rows), "two phases × positions {5,6}")
	// check that per-phase rows exist
	pp := 0
	death := 0
	for i := range rows {
		if rows[i].Phase == "powerplay" {
			pp++
		}
		if rows[i].Phase == "death" {
			death++
		}
	}
	require.Equal(t, 2, pp)
	require.Equal(t, 2, death)
}

// 1.13.1 — Multi-format mapping tests for end_pressure
func TestAggregateEndPressure_FormatMapping(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 9, 14, 0, 0, 0, 0, time.UTC)
	bow := int64(8181)
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
			seq := []evRowEP{
				// Over 10 with six legal balls so positions 5 and 6 exist
				evEP(10, 1, 10, 1, "middle", true, bow, 0, 0, 0, asOf, tc.fmtID),   // pos1
				evEP(10, 1, 10, 2, "middle", true, bow, 0, 0, 0, asOf, tc.fmtID),   // pos2
				evEP(10, 1, 10, 3, "middle", true, bow, 0, 0, 0, asOf, tc.fmtID),   // pos3
				evEP(10, 1, 10, 4, "middle", true, bow, 0, 0, 0, asOf, tc.fmtID),   // pos4
				evEP(10, 1, 10, 5, "middle", true, bow, 4, 4, 0, asOf, tc.fmtID),   // pos5 boundary
				evEP(10, 1, 10, 6, "middle", true, bow, 0, 0, 123, asOf, tc.fmtID), // pos6 wicket
			}
			// Act
			rows := aggregateEndPressure(seq)
			// Assert
			require.NotEmpty(t, rows, "expected rows for format %d", tc.fmtID)
			found5 := false
			found6 := false
			for i := range rows {
				if rows[i].FormatID == tc.fmtID && rows[i].PlayerID == bow && rows[i].Position == 5 {
					found5 = true
				}
				if rows[i].FormatID == tc.fmtID && rows[i].PlayerID == bow && rows[i].Position == 6 {
					found6 = true
				}
			}
			require.Truef(
				t,
				found5 && found6,
				"did not find both positions for fmt %d: p5=%v p6=%v",
				tc.fmtID,
				found5,
				found6,
			)
		})
	}
}
