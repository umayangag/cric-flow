package seqcalc

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func nullInt(v int64) sql.NullInt64   { return sql.NullInt64{Int64: v, Valid: true} }
func nullStr(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }

func TestMakeBatAgg(t *testing.T) {
	const striker int64 = 7

	testCases := []struct {
		name  string
		event bEvent
		want  pwBatAgg
	}{
		{
			name:  "single off the bat",
			event: bEvent{RunsBatter: 1, RunsTotal: 1},
			want:  pwBatAgg{balls: 1, runs: 1},
		},
		{
			name:  "four counts as a boundary",
			event: bEvent{RunsBatter: 4, RunsTotal: 4},
			want:  pwBatAgg{balls: 1, runs: 4, fours: 1},
		},
		{
			name:  "six counts as a boundary",
			event: bEvent{RunsBatter: 6, RunsTotal: 6},
			want:  pwBatAgg{balls: 1, runs: 6, sixes: 1},
		},
		{
			name:  "dot ball",
			event: bEvent{RunsBatter: 0, RunsTotal: 0},
			want:  pwBatAgg{balls: 1, dots: 1},
		},
		{
			// dots key on RunsTotal, not RunsBatter: a bye off a no-score ball
			// still concedes runs, so it is not a dot for the batter's window.
			name:  "no runs off the bat but extras conceded is not a dot",
			event: bEvent{RunsBatter: 0, RunsTotal: 1, ExtrasKind: nullStr("byes")},
			want:  pwBatAgg{balls: 1, runs: 0},
		},
		{
			name:  "striker dismissed",
			event: bEvent{RunsTotal: 0, WicketKind: nullStr("bowled"), PlayerOutID: nullInt(striker)},
			want:  pwBatAgg{balls: 1, dots: 1, dismissals: 1},
		},
		{
			// a run-out at the non-striker's end must not be charged to this batter
			name:  "non-striker dismissed is not charged to the striker",
			event: bEvent{RunsBatter: 1, RunsTotal: 1, WicketKind: nullStr("run out"), PlayerOutID: nullInt(99)},
			want:  pwBatAgg{balls: 1, runs: 1},
		},
		{
			name:  "wicket kind present but no player out recorded",
			event: bEvent{RunsTotal: 0, WicketKind: nullStr("retired hurt")},
			want:  pwBatAgg{balls: 1, dots: 1},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, makeBatAgg(tc.event, striker))
		})
	}
}

func TestMakeBowlAgg(t *testing.T) {
	testCases := []struct {
		name  string
		event bEvent
		want  pwBowlAgg
	}{
		{
			name:  "single conceded",
			event: bEvent{RunsBatter: 1, RunsTotal: 1},
			want:  pwBowlAgg{balls: 1, runs: 1},
		},
		{
			name:  "dot ball",
			event: bEvent{RunsBatter: 0, RunsTotal: 0},
			want:  pwBowlAgg{balls: 1, dotBalls: 1},
		},
		{
			name:  "four conceded",
			event: bEvent{RunsBatter: 4, RunsTotal: 4},
			want:  pwBowlAgg{balls: 1, runs: 4, boundariesConceded: 1},
		},
		{
			name:  "six conceded",
			event: bEvent{RunsBatter: 6, RunsTotal: 6},
			want:  pwBowlAgg{balls: 1, runs: 6, boundariesConceded: 1},
		},
		{
			// boundaries count only off the bat: four byes are runs, not a boundary conceded
			name:  "four byes are runs but not a boundary conceded",
			event: bEvent{RunsBatter: 0, RunsTotal: 4, ExtrasKind: nullStr("byes")},
			want:  pwBowlAgg{balls: 1, runs: 4},
		},
		{
			name:  "wicket credited",
			event: bEvent{RunsTotal: 0, WicketKind: nullStr("caught"), PlayerOutID: nullInt(3)},
			want:  pwBowlAgg{balls: 1, dotBalls: 1, wickets: 1},
		},
		{
			name:  "wicket kind without player out is not credited",
			event: bEvent{RunsTotal: 0, WicketKind: nullStr("caught")},
			want:  pwBowlAgg{balls: 1, dotBalls: 1},
		},
		{
			// documented behaviour: wides/no-balls are illegal deliveries and are
			// excluded from horizon windows, so wideNB is always left at zero here
			name:  "wideNB is never set by this helper",
			event: bEvent{RunsBatter: 0, RunsTotal: 1, ExtrasKind: nullStr("wides")},
			want:  pwBowlAgg{balls: 1, runs: 1},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, makeBowlAgg(tc.event))
		})
	}
}

func newBatState(horizons []int) *pwBatState {
	st := &pwBatState{
		deques: make(map[int]*pwBatDeque, len(horizons)),
		last:   make(map[int]pwBatAgg, len(horizons)),
		phase:  make(map[int]string, len(horizons)),
	}
	for _, h := range horizons {
		st.deques[h] = &pwBatDeque{}
	}
	return st
}

func newBowlState(horizons []int) *pwBowlState {
	st := &pwBowlState{
		deques: make(map[int]*pwBowlDeque, len(horizons)),
		last:   make(map[int]pwBowlAgg, len(horizons)),
		phase:  make(map[int]string, len(horizons)),
	}
	for _, h := range horizons {
		st.deques[h] = &pwBowlDeque{}
	}
	return st
}

func TestUpdateBatState_HorizonsEvictIndependently(t *testing.T) {
	horizons := []int{2, 5}
	st := newBatState(horizons)

	// four scoring balls: 4, 6, 1, 0
	balls := []pwBatAgg{
		{balls: 1, runs: 4, fours: 1},
		{balls: 1, runs: 6, sixes: 1},
		{balls: 1, runs: 1},
		{balls: 1, dots: 1},
	}
	for i, b := range balls {
		phase := "powerplay"
		if i >= 2 {
			phase = "middle"
		}
		updateBatState(st, bEvent{Phase: phase}, horizons, b)
	}

	// horizon 2 keeps only the last two balls: 1 run and a dot
	require.Equal(t, pwBatAgg{balls: 2, runs: 1, dots: 1}, st.last[2])
	// horizon 5 is wider than the number of balls, so it keeps all four
	require.Equal(t, pwBatAgg{balls: 4, runs: 11, fours: 1, sixes: 1, dots: 1}, st.last[5])
	// phase is recorded per horizon from the most recent ball
	require.Equal(t, "middle", st.phase[2])
	require.Equal(t, "middle", st.phase[5])
}

func TestUpdateBowlState_HorizonsEvictIndependently(t *testing.T) {
	horizons := []int{3, 10}
	st := newBowlState(horizons)

	balls := []pwBowlAgg{
		{balls: 1, runs: 4, boundariesConceded: 1},
		{balls: 1, dotBalls: 1},
		{balls: 1, runs: 2},
		{balls: 1, wickets: 1, dotBalls: 1},
	}
	for _, b := range balls {
		updateBowlState(st, bEvent{Phase: "death"}, horizons, b)
	}

	// horizon 3 drops the opening four
	require.Equal(t, pwBowlAgg{balls: 3, runs: 2, wickets: 1, dotBalls: 2}, st.last[3])
	// horizon 10 retains everything
	require.Equal(t, pwBowlAgg{balls: 4, runs: 6, wickets: 1, dotBalls: 2, boundariesConceded: 1}, st.last[10])
	require.Equal(t, "death", st.phase[3])
	require.Equal(t, "death", st.phase[10])
}

func TestUpdateBatState_NoHorizonsIsANoOp(t *testing.T) {
	st := newBatState(nil)
	updateBatState(st, bEvent{Phase: "powerplay"}, nil, pwBatAgg{balls: 1, runs: 4})
	require.Empty(t, st.last)
	require.Empty(t, st.phase)
}

func TestBuildRowsForInnings(t *testing.T) {
	opp := int64(11)
	venue := int64(22)
	season := int64(33)

	batStates := map[int64]*pwBatState{
		101: {
			last:  map[int]pwBatAgg{12: {balls: 10, runs: 20, fours: 2, sixes: 1, dots: 4, dismissals: 1}},
			phase: map[int]string{12: "powerplay"},
		},
	}
	bowlStates := map[int64]*pwBowlState{
		202: {
			last:  map[int]pwBowlAgg{24: {balls: 12, runs: 18, wickets: 2, dotBalls: 5, boundariesConceded: 1}},
			phase: map[int]string{24: "death"},
		},
	}

	rows := buildRowsForInnings(
		"2024-05-01", 3, &opp, &venue, &season,
		[]int{12}, []int{24}, batStates, bowlStates,
	)

	// one batter x one horizon x {overall, opposition, venue, season} = 4,
	// plus the same for the bowler = 8
	require.Len(t, rows, 8)

	byScope := map[string]db.PlayerWindowRow{}
	for _, r := range rows {
		if r.Role == "bat" {
			byScope["bat/"+r.Scope] = r
		} else {
			byScope["bowl/"+r.Scope] = r
		}
	}
	require.Len(t, byScope, 8)

	bat := byScope["bat/overall"]
	require.Equal(t, "2024-05-01", bat.AsOfDate)
	require.Equal(t, 3, bat.FormatID)
	require.Equal(t, int64(101), bat.PlayerID)
	require.Equal(t, "powerplay", bat.Phase)
	require.Equal(t, 12, bat.Horizon)
	require.Nil(t, bat.ScopeID, "overall scope carries no scope id")
	require.Equal(t, 20, bat.Runs)
	// derived rates: SR = runs*100/balls, boundary = (4s+6s)/balls, dot = dots/balls
	require.NotNil(t, bat.SR)
	require.InDelta(t, 200.0, *bat.SR, 1e-9)
	require.InDelta(t, 0.3, *bat.BoundaryRate, 1e-9)
	require.InDelta(t, 0.4, *bat.DotRate, 1e-9)
	require.InDelta(t, 0.1, *bat.DismissalHaz, 1e-9)
	// bowling-only fields stay zero on a batting row
	require.Zero(t, bat.Wickets)
	require.Nil(t, bat.Econ)

	bowl := byScope["bowl/overall"]
	require.Equal(t, int64(202), bowl.PlayerID)
	require.Equal(t, "death", bowl.Phase)
	// econ = runs*6/balls
	require.NotNil(t, bowl.Econ)
	require.InDelta(t, 9.0, *bowl.Econ, 1e-9)
	require.InDelta(t, 5.0/12.0, *bowl.DotRate, 1e-9)
	require.InDelta(t, 2.0/12.0, *bowl.WicketRate, 1e-9)
	require.Nil(t, bowl.SR, "batting-only rates stay unset on a bowling row")

	// scoped rows carry the scope id through
	require.Equal(t, &opp, byScope["bat/opposition"].ScopeID)
	require.Equal(t, &venue, byScope["bowl/venue"].ScopeID)
	require.Equal(t, &season, byScope["bowl/season"].ScopeID)
}

func TestBuildRowsForInnings_NilScopesEmitOverallOnly(t *testing.T) {
	batStates := map[int64]*pwBatState{
		1: {last: map[int]pwBatAgg{6: {balls: 6, runs: 6}}, phase: map[int]string{6: "middle"}},
	}
	rows := buildRowsForInnings("2024-01-01", 1, nil, nil, nil, []int{6}, nil, batStates, nil)
	require.Len(t, rows, 1)
	require.Equal(t, "overall", rows[0].Scope)
}

func TestBuildRowsForInnings_ZeroBallsLeavesRatesUnset(t *testing.T) {
	batStates := map[int64]*pwBatState{
		1: {last: map[int]pwBatAgg{6: {}}, phase: map[int]string{6: ""}},
	}
	bowlStates := map[int64]*pwBowlState{
		2: {last: map[int]pwBowlAgg{6: {}}, phase: map[int]string{6: ""}},
	}
	rows := buildRowsForInnings("2024-01-01", 1, nil, nil, nil, []int{6}, []int{6}, batStates, bowlStates)
	require.Len(t, rows, 2)
	for _, r := range rows {
		// guarding against a divide-by-zero: no balls means no derived rate
		require.Nil(t, r.SR)
		require.Nil(t, r.Econ)
		require.Nil(t, r.DotRate)
	}
}

func TestBuildRowsForInnings_HorizonWithoutAggregateIsSkipped(t *testing.T) {
	batStates := map[int64]*pwBatState{
		1: {last: map[int]pwBatAgg{6: {balls: 3, runs: 4}}, phase: map[int]string{6: "middle"}},
	}
	// horizon 30 has no recorded aggregate for this innings
	rows := buildRowsForInnings("2024-01-01", 1, nil, nil, nil, []int{6, 30}, nil, batStates, nil)
	require.Len(t, rows, 1)
	require.Equal(t, 6, rows[0].Horizon)
}
