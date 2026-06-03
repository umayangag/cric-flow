package seqcalc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPushBatAndSum(t *testing.T) {
	// window of 3, push 4 items; first should drop
	d := &pwBatDeque{}
	pushBat(d, pwBatAgg{balls: 1, runs: 2, fours: 0, sixes: 0, dots: 1, dismissals: 0}, 3)
	pushBat(d, pwBatAgg{balls: 1, runs: 4, fours: 1, sixes: 0, dots: 0, dismissals: 0}, 3)
	pushBat(d, pwBatAgg{balls: 1, runs: 6, fours: 0, sixes: 1, dots: 0, dismissals: 1}, 3)
	pushBat(d, pwBatAgg{balls: 1, runs: 0, fours: 0, sixes: 0, dots: 1, dismissals: 0}, 3)
	require.Len(t, d.items, 3)
	s := batSum(d)
	// We expect the last 3 items: (4,6,0) runs -> total 10; balls 3; fours 1; sixes 1; dots 0+0+1=1; dismissals 1
	require.Equal(t, 3, s.balls)
	require.Equal(t, 10, s.runs)
	require.Equal(t, 1, s.fours)
	require.Equal(t, 1, s.sixes)
	require.Equal(t, 1, s.dots)
	require.Equal(t, 1, s.dismissals)
}

func TestPushBowlAndSum(t *testing.T) {
	// window of 2, push 3 items; first should drop
	d := &pwBowlDeque{}
	pushBowl(d, pwBowlAgg{balls: 1, runs: 2, wickets: 0, dotBalls: 1, boundariesConceded: 0, wideNB: 0}, 2)
	pushBowl(d, pwBowlAgg{balls: 1, runs: 4, wickets: 1, dotBalls: 0, boundariesConceded: 1, wideNB: 0}, 2)
	pushBowl(d, pwBowlAgg{balls: 1, runs: 0, wickets: 0, dotBalls: 1, boundariesConceded: 0, wideNB: 0}, 2)
	require.Len(t, d.items, 2)
	s := bowlSum(d)
	// Expect last 2 items: runs 4+0=4; balls 2; wickets 1; dotBalls 0+1=1; boundaries 1; wideNB 0
	require.Equal(t, 2, s.balls)
	require.Equal(t, 4, s.runs)
	require.Equal(t, 1, s.wickets)
	require.Equal(t, 1, s.dotBalls)
	require.Equal(t, 1, s.boundariesConceded)
	require.Equal(t, 0, s.wideNB)
}
