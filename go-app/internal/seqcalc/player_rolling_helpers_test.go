package seqcalc

import (
	"testing"
)

func TestPushBatAndSum(t *testing.T) {
	// window of 3, push 4 items; first should drop
	d := &pwBatDeque{}
	pushBat(d, pwBatAgg{balls: 1, runs: 2, fours: 0, sixes: 0, dots: 1, dismissals: 0}, 3)
	pushBat(d, pwBatAgg{balls: 1, runs: 4, fours: 1, sixes: 0, dots: 0, dismissals: 0}, 3)
	pushBat(d, pwBatAgg{balls: 1, runs: 6, fours: 0, sixes: 1, dots: 0, dismissals: 1}, 3)
	pushBat(d, pwBatAgg{balls: 1, runs: 0, fours: 0, sixes: 0, dots: 1, dismissals: 0}, 3)
	if len(d.items) != 3 {
		t.Fatalf("expected deque length 3, got %d", len(d.items))
	}
	s := batSum(d)
	// We expect the last 3 items: (4,6,0) runs -> total 10; balls 3; fours 1; sixes 1; dots 0+0+1=1; dismissals 1
	if s.balls != 3 || s.runs != 10 || s.fours != 1 || s.sixes != 1 || s.dots != 1 || s.dismissals != 1 {
		t.Fatalf("unexpected batSum: %+v", s)
	}
}

func TestPushBowlAndSum(t *testing.T) {
	// window of 2, push 3 items; first should drop
	d := &pwBowlDeque{}
	pushBowl(d, pwBowlAgg{balls: 1, runs: 2, wickets: 0, dotBalls: 1, boundariesConceded: 0, wideNB: 0}, 2)
	pushBowl(d, pwBowlAgg{balls: 1, runs: 4, wickets: 1, dotBalls: 0, boundariesConceded: 1, wideNB: 0}, 2)
	pushBowl(d, pwBowlAgg{balls: 1, runs: 0, wickets: 0, dotBalls: 1, boundariesConceded: 0, wideNB: 0}, 2)
	if len(d.items) != 2 {
		t.Fatalf("expected deque length 2, got %d", len(d.items))
	}
	s := bowlSum(d)
	// Expect last 2 items: runs 4+0=4; balls 2; wickets 1; dotBalls 0+1=1; boundaries 1; wideNB 0
	if s.balls != 2 || s.runs != 4 || s.wickets != 1 || s.dotBalls != 1 || s.boundariesConceded != 1 || s.wideNB != 0 {
		t.Fatalf("unexpected bowlSum: %+v", s)
	}
}
