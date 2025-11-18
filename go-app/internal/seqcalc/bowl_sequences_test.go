package seqcalc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPairConsecutiveOvers_BasicSequences follows the gold standard: AAA with require assertions.
func TestPairConsecutiveOvers_BasicSequences(t *testing.T) {
	t.Parallel()

	// Arrange: Over order from tests/fixtures/seq/t20/overs_order.json
	// Overs 1..6 bowler IDs: 201,202,201,203,202,203
	seq := []overSummary{
		{matchID: 2, innings: 1, over: 1, bowler: 201, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 2, bowler: 202, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 3, bowler: 201, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 4, bowler: 203, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 5, bowler: 202, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
		{matchID: 2, innings: 1, over: 6, bowler: 203, phase: "powerplay", date: "2025-01-01", format: 3, balls: 6},
	}

	// Act
	pairs := pairConsecutiveOvers(seq)

	// Assert
	require.Len(t, pairs, 5, "Expect 5 consecutive pairs from 6 overs")

	check := func(prev, curr int64) {
		found := false
		for i := range pairs {
			if pairs[i].prevBowler == prev && pairs[i].currBowler == curr {
				require.Equalf(t, 1, pairs[i].oversPairs, "pair %d→%d oversPairs", prev, curr)
				found = true
				break
			}
		}
		require.Truef(t, found, "expected pair %d→%d not found", prev, curr)
	}
	check(201, 202)
	check(202, 201)
	check(201, 203)
	check(203, 202)
	check(202, 203)
}
