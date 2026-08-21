package cricsheet_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

// TestStableMatchID_Table follows the gold-standard: table-driven, AAA, require assertions.
func TestStableMatchID_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func() (a, b string, team1, team2 string)
	type assertFn func(t *testing.T, base string, same string, diffOrder string, diffDate string)

	testCases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "deterministic and sensitive to order and date",
			arrange: func() (string, string, string, string) {
				return "2025-11-07", "2025-11-07", "India", "Australia"
			},
			assert: func(t *testing.T, base, same, _, _ string) {
				id1 := cricsheet.StableMatchID(base, "India", "Australia")
				id2 := cricsheet.StableMatchID(same, "India", "Australia")
				idOrder := cricsheet.StableMatchID(base, "Australia", "India")
				idDate := cricsheet.StableMatchID("2025-11-08", "India", "Australia")
				require.Equal(t, id1, id2)
				require.NotEqual(t, id1, idOrder)
				require.NotEqual(t, id1, idDate)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			base, same, team1, team2 := tc.arrange()
			_ = team1
			_ = team2
			tc.assert(t, base, same, team1, team2)
		})
	}
}

func TestStableMatchID_EdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		dateISO string
		teamA   string
		teamB   string
	}{
		{"same_teams_same_date", "2025-01-01", "India", "Australia"},
		{"empty_date", "", "A", "B"},
		{"empty_teams", "2025-01-01", "", ""},
		{"order_matters", "2025-01-01", "India", "Australia"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			id := cricsheet.StableMatchID(tc.dateISO, tc.teamA, tc.teamB)
			if tc.dateISO != "" && tc.teamA != "" && tc.teamB != "" {
				id2 := cricsheet.StableMatchID(tc.dateISO, tc.teamA, tc.teamB)
				require.Equal(t, id, id2, "same inputs must yield same ID")
			}
			// Swap order must differ when both teams non-empty
			if tc.teamA != "" && tc.teamB != "" && tc.teamA != tc.teamB {
				idSwap := cricsheet.StableMatchID(tc.dateISO, tc.teamB, tc.teamA)
				require.NotEqual(t, id, idSwap, "swapped teams must yield different ID")
			}
		})
	}
}
