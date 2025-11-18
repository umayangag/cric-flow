package seqcalc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// helper to craft evRowWK rows
//
//nolint:unparam // helper accepts many params for clarity; some are constant in tests
func evWK(
	match int64,
	inng, ballSeq int,
	phase string,
	isLegal bool,
	bowler int64,
	runsTot int,
	extrasKind string,
	outPID int64,
	wicketKind string,
	asOf time.Time,
	fmtID int,
) evRowWK {
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
	t.Parallel()
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
		require.Equalf(t, tc.want, got, "canonicalMode(%q)", tc.in)
	}
	// empty/null
	require.Equal(t, "", canonicalMode(sql.NullString{}))
}

func TestAggregateWicketModes_Basic(t *testing.T) {
	t.Parallel()
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
	require.Equal(t, 1, len(agg))
	r := agg[0]
	require.Equal(t, bowler, r.PlayerID)
	require.Equal(t, fmtID, r.FormatID)
	require.Equal(t, "powerplay", r.Phase)
	require.Equal(t, "caught", r.Mode)
	require.Equal(t, 3, r.Balls) // only legal deliveries counted
	require.Equal(t, 1, r.Wickets)
	require.Greater(t, r.WicketsPer100, 0.0)
	require.LessOrEqual(t, r.WicketsPer100, 40.0)
}

func TestAggregateWicketModes_PhaseSeparation(t *testing.T) {
	t.Parallel()
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
	require.Equal(t, 2, len(agg))
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
	require.NotNil(t, pp, "missing powerplay agg: %+v", agg)
	require.NotNil(t, death, "missing death agg: %+v", agg)
	require.Equal(t, 2, pp.Balls)
	require.Equal(t, 0, pp.Wickets)
	require.Equal(t, 2, death.Balls)
	require.Equal(t, 1, death.Wickets)
	require.Equal(t, "lbw", death.Mode)
}

func TestAggregateWicketModes_FormatMapping(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 9, 11, 0, 0, 0, 0, time.UTC)
	bowler := int64(4242)
	cases := []struct {
		name  string
		fmtID int
	}{
		{"ODI", 2},
		{"TEST", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := []evRowWK{
				// two legal balls, one wicket in given format
				evWK(9, 1, 1, "middle", true, bowler, 0, "", 0, "", asOf, tc.fmtID),
				evWK(9, 1, 2, "middle", true, bowler, 0, "", 999, "lbw", asOf, tc.fmtID),
			}
			agg := aggregateWicketModes(rows)
			require.NotEmpty(t, agg, "expected rows for format %d", tc.fmtID)
			found := false
			for i := range agg {
				if agg[i].FormatID == tc.fmtID && agg[i].PlayerID == bowler {
					found = true
					break
				}
			}
			require.True(t, found, "did not find aggregated row for fmt %d", tc.fmtID)
		})
	}
}
