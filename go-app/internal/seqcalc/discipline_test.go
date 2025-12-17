package seqcalc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// evD is similar to ev/ev2 helpers in sibling tests; kept local for clarity.
//
//nolint:unparam // helper accepts many params for clarity; some are constant in tests
func evD(
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

// TestAggregateDiscipline_BasicCountsAndRates follows AAA with require assertions.
func TestAggregateDiscipline_BasicCountsAndRates(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	bowler := int64(211)
	// Build one over (6 legal balls) plus one illegal wide in between. Phase: powerplay.
	seq := []evRow{
		// 1: legal dot
		evD(1, 1, 1, "powerplay", true, 111, bowler, 0, 0, "", 0, asOf, fmtID),
		// 2: legal single
		evD(1, 1, 2, "powerplay", true, 111, bowler, 1, 1, "", 0, asOf, fmtID),
		// 3: illegal wide (adds 1 run to total, does not increment balls_bowled)
		evD(1, 1, 3, "powerplay", false, 111, bowler, 0, 1, "wide", 0, asOf, fmtID),
		// 4: legal 4
		evD(1, 1, 4, "powerplay", true, 111, bowler, 4, 4, "", 0, asOf, fmtID),
		// 5: legal no-ball (treated as extra, but we set runs_total=1 here and mark extras kind)
		evD(1, 1, 5, "powerplay", false, 111, bowler, 0, 1, "no_ball", 0, asOf, fmtID),
		// 6: legal dot
		evD(1, 1, 6, "powerplay", true, 111, bowler, 0, 0, "", 0, asOf, fmtID),
		// 7: legal wicket (striker out)
		evD(1, 1, 7, "powerplay", true, 111, bowler, 0, 0, "", 111, asOf, fmtID),
		// 8: legal leg-bye (1 run totals)
		evD(1, 1, 8, "powerplay", true, 113, bowler, 0, 1, "leg_bye", 0, asOf, fmtID),
	}

	// Act
	rows := aggregateDiscipline(seq)

	// Assert
	require.Equal(t, 1, len(rows))
	r := rows[0]
	require.Equal(t, bowler, r.PlayerID)
	require.Equal(t, fmtID, r.FormatID)
	require.Equal(t, "powerplay", r.Phase)
	// balls_bowled counts only legal balls: legal are 1,2,4,6,7,8 => 6
	require.Equal(t, 6, r.BallsBowled)
	// overs is integer division of balls_bowled/6 => 1
	require.Equal(t, 1, r.Overs)
	// extras counts
	require.Equal(t, 1, r.Wides)
	require.Equal(t, 1, r.NoBalls)
	require.Equal(t, 1, r.LegByes)
	require.Equal(t, 0, r.Byes)
	require.Equal(t, 0, r.PenaltyRuns)
	require.Equal(t, r.Wides+r.NoBalls+r.Byes+r.LegByes+r.PenaltyRuns, r.ExtrasTotal)
	// wickets
	require.Equal(t, 1, r.Wickets)
	// runs_conceded is sum of runs_total across all deliveries (including illegal extras): 0+1+1+4+1+0+0+1 = 8
	require.Equal(t, 8, r.RunsConceded)
	// Rates per over with overs=1
	require.Equal(t, 1.0, r.WidesPerOver)
	require.Equal(t, 1.0, r.NoBallsPerOver)
	require.Equal(t, float64(r.ExtrasTotal), r.ExtrasPerOver)
}

func TestAggregateDiscipline_FormatMapping(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 9, 10, 0, 0, 0, 0, time.UTC)
	bowler := int64(909)
	cases := []struct {
		name  string
		fmtID int
	}{
		{"ODI", 2},
		{"TEST", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seq := []evRow{
				// two legal balls in powerplay
				evD(10, 1, 1, "powerplay", true, 101, bowler, 0, 0, "", 0, asOf, tc.fmtID),
				evD(10, 1, 2, "powerplay", true, 101, bowler, 1, 1, "", 0, asOf, tc.fmtID),
			}
			rows := aggregateDiscipline(seq)
			require.NotEmpty(t, rows)
			r := rows[0]
			require.Equal(t, tc.fmtID, r.FormatID)
			require.Equal(t, bowler, r.PlayerID)
		})
	}
}
