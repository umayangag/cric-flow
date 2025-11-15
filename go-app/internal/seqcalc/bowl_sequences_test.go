package seqcalc

import "testing"

func TestPairConsecutiveOvers_BasicSequences(t *testing.T) {
	// Over order from tests/fixtures/seq/t20/overs_order.json
	// Overs 1..6 bowler IDs: 201,202,201,203,202,203
	seq := []overSummary{
		{matchID: 2, innings: 1, over: 1, bowler: 201, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 2, bowler: 202, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 3, bowler: 201, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 4, bowler: 203, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 5, bowler: 202, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 6, bowler: 203, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
	}
	pairs := pairConsecutiveOvers(seq)
	if len(pairs) != 5 {
		// Expect 5 consecutive pairs from 6 overs
		t.Fatalf("unexpected pair count: got %d want 5", len(pairs))
	}
	check := func(prev, curr int64) {
		found := false
		for _, p := range pairs {
			if p.prevBowler == prev && p.currBowler == curr {
				if p.oversPairs != 1 {
					t.Fatalf("pair %d→%d oversPairs=%d want 1", prev, curr, p.oversPairs)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected pair %d→%d not found", prev, curr)
		}
	}
	check(201, 202)
	check(202, 201)
	check(201, 203)
	check(203, 202)
	check(202, 203)
}
