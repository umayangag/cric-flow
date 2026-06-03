package teamselect_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

type assertSelFn func(t *testing.T, team []ts.Player, err error)

func assertNoErrorSize(want int, wantKeeper bool, wantBowlers int) assertSelFn {
	return func(t *testing.T, team []ts.Player, err error) {
		require.NoError(t, err)
		require.Len(t, team, want)
		if wantKeeper {
			require.GreaterOrEqual(
				t,
				countIf(team, func(p ts.Player) bool { return p.IsKeeper }),
				1,
				"expected a keeper in team",
			)
		}
		require.GreaterOrEqual(t, countIf(team, func(p ts.Player) bool { return p.IsBowler }), wantBowlers, "bowlers")
	}
}

func assertErrContains(sub string) assertSelFn {
	return func(t *testing.T, _ []ts.Player, err error) {
		require.Error(t, err)
		require.Contains(t, err.Error(), sub)
	}
}

func countIf(ps []ts.Player, pred func(ts.Player) bool) int {
	n := 0
	for _, p := range ps {
		if pred(p) {
			n++
		}
	}
	return n
}

func TestSelect_Table(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	mk := func(name string, bat, bowl float64, isBow, isKeep bool) ts.Player {
		return ts.Player{Name: name, BatScore: bat, BowlScore: bowl, IsBowler: isBow, IsKeeper: isKeep}
	}
	pool := []ts.Player{
		mk("A", 0.9, 0.1, false, false),
		mk("B", 0.7, 0.8, true, false),
		mk("C", 0.6, 0.7, true, false),
		mk("D", 0.5, 0.2, false, false),
		mk("E", 0.4, 0.9, true, false),
		mk("K", 0.3, 0.3, false, true), // keeper
	}
	testCases := []struct {
		name   string
		pool   []ts.Player
		c      ts.Constraints
		assert assertSelFn
	}{
		{
			name:   "happy path no extra constraints",
			pool:   pool,
			c:      ts.Constraints{Size: 4, MinBowlers: 0, RequireKeeper: false},
			assert: assertNoErrorSize(4, false, 0),
		},
		{
			name:   "require keeper satisfied via swap",
			pool:   pool,
			c:      ts.Constraints{Size: 4, MinBowlers: 1, RequireKeeper: true},
			assert: assertNoErrorSize(4, true, 1),
		},
		{
			name:   "enforce min bowlers via swaps",
			pool:   pool,
			c:      ts.Constraints{Size: 5, MinBowlers: 3, RequireKeeper: false},
			assert: assertNoErrorSize(5, false, 3),
		},
		{
			name: "insufficient bowlers errors",
			pool: []ts.Player{
				mk("A", 1, 0, false, false),
				mk("B", 0.9, 0, false, false),
				mk("K", 0.1, 0, false, true),
			},
			c:      ts.Constraints{Size: 3, MinBowlers: 1, RequireKeeper: false},
			assert: assertErrContains("not enough bowlers"),
		},
		{
			name:   "insufficient pool size",
			pool:   []ts.Player{mk("A", 1, 0, false, false)},
			c:      ts.Constraints{Size: 2, MinBowlers: 0, RequireKeeper: false},
			assert: assertErrContains("insufficient pool"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			team, err := ts.Select(tc.pool, w, tc.c)
			// delegate checks to the assert function
			tc.assert(t, team, err)
		})
	}
}
