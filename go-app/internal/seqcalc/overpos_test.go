package seqcalc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// helper to craft ev rows for over position tests
//
//nolint:unparam // helper accepts many params for clarity; some are constant in tests
func evOP(
	match int64,
	inng, over, ball int,
	phase string,
	isLegal bool,
	bowler int64,
	runsBat int,
	outPID int64,
	asOf time.Time,
	fmtID int,
) evRowOverPos {
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

// TestAggregateOverPos_Basic follows the gold standard: AAA style with require assertions.
func TestAggregateOverPos_Basic(t *testing.T) {
	t.Parallel()
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
	// Act
	agg := aggregateOverPos(rows)

	// Assert
	require.Equal(t, 2, len(agg), "expected 2 rows (pos 1 and 6)")
	var pos1, pos6 *overPosRow
	for i := range agg {
		if agg[i].Position == 1 {
			pos1 = &agg[i]
		}
		if agg[i].Position == 6 {
			pos6 = &agg[i]
		}
	}
	require.NotNil(t, pos1, "missing position 1: %+v", agg)
	require.NotNil(t, pos6, "missing position 6: %+v", agg)
	require.Equal(t, 2, pos1.Balls)
	require.Equal(t, 1, pos1.Boundaries)
	require.Equal(t, 0, pos1.Wickets)
	require.Equal(t, 2, pos6.Balls)
	require.Equal(t, 1, pos6.Boundaries)
	require.Equal(t, 1, pos6.Wickets)
	require.Greater(t, pos1.BoundaryRate, 0.0)
	require.Less(t, pos1.BoundaryRate, 1.0)
	require.Greater(t, pos6.WicketRate, 0.0)
	require.Less(t, pos6.WicketRate, 1.0)
}

func TestAggregateOverPos_IllegalNotCounted(t *testing.T) {
	t.Parallel()
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
	// Act
	agg := aggregateOverPos(rows)

	// Assert
	require.Equal(t, 2, len(agg))
	var p1, p6 *overPosRow
	for i := range agg {
		if agg[i].Position == 1 {
			p1 = &agg[i]
		}
		if agg[i].Position == 6 {
			p6 = &agg[i]
		}
	}
	require.NotNil(t, p1)
	require.NotNil(t, p6)
	require.Equal(t, 1, p1.Balls)
	require.Equal(t, 0, p1.Boundaries)
	require.Equal(t, 0, p1.Wickets)
	require.Equal(t, 1, p6.Balls)
	require.Equal(t, 0, p6.Boundaries)
	require.Equal(t, 1, p6.Wickets)
}

func TestAggregateOverPos_PhaseSeparation(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 8, 12, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bow := int64(888)
	rows := []evRowOverPos{
		evOP(3, 1, 5, 1, "powerplay", true, bow, 0, 0, asOf, fmtID),
		evOP(3, 1, 5, 6, "powerplay", true, bow, 6, 0, asOf, fmtID),
		evOP(3, 1, 17, 1, "death", true, bow, 4, 0, asOf, fmtID),
	}
	// Act
	agg := aggregateOverPos(rows)
	// Assert
	require.Equal(t, 3, len(agg), "2 powerplay (pos 1 & 6), 1 death (pos 1)")
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
	require.True(t, pp1 && pp6 && d1, "phase separation missing: %+v", agg)
}

// 1.13.1 — Multi-format mapping tests for overpos
func TestAggregateOverPos_FormatMapping(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 9, 13, 0, 0, 0, 0, time.UTC)
	bow := int64(9090)
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
			rows := []evRowOverPos{
				// one legal ball at position 1 for simplicity
				evOP(7, 1, 3, 1, "middle", true, bow, 0, 0, asOf, tc.fmtID),
			}
			// Act
			agg := aggregateOverPos(rows)
			// Assert
			require.NotEmpty(t, agg, "expected rows for format %d", tc.fmtID)
			found := false
			for i := range agg {
				if agg[i].FormatID == tc.fmtID && agg[i].PlayerID == bow {
					found = true
					break
				}
			}
			require.True(t, found, "did not find aggregated row for fmt %d", tc.fmtID)
		})
	}
}
