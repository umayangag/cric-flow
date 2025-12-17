package cricsheet_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
)

// TestStableMatchID_Table follows the gold-standard: table-driven, AAA, require assertions.
func TestStableMatchID_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func() (a, b string, team1, team2 string)
	type assertFn func(t *testing.T, base string, same string, diffOrder string, diffDate string)

	cases := []struct {
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
				// Arrange
				// Act
				id1 := cricsheet.StableMatchID(base, "India", "Australia")
				id2 := cricsheet.StableMatchID(same, "India", "Australia")
				idOrder := cricsheet.StableMatchID(base, "Australia", "India")
				idDate := cricsheet.StableMatchID("2025-11-08", "India", "Australia")
				// Assert
				require.Equal(t, id1, id2)
				require.NotEqual(t, id1, idOrder)
				require.NotEqual(t, id1, idDate)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, same, team1, team2 := tc.arrange()
			_ = team1
			_ = team2
			tc.assert(t, base, same, team1, team2)
		})
	}
}
