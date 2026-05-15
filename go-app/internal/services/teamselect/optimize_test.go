package teamselect_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// mkPool returns a pool of 11 players: 6 batters, 5 bowlers, 1 keeper.
func mkPool() []ts.Player {
	return []ts.Player{
		{Name: "A", BatScore: 0.9, IsBowler: false, IsKeeper: true},
		{Name: "B", BatScore: 0.85, IsBowler: false},
		{Name: "C", BatScore: 0.8, IsBowler: false},
		{Name: "D", BatScore: 0.75, IsBowler: false},
		{Name: "E", BatScore: 0.7, IsBowler: false},
		{Name: "F", BatScore: 0.65, IsBowler: false},
		{Name: "G", BowlScore: 0.9, IsBowler: true},
		{Name: "H", BowlScore: 0.8, IsBowler: true},
		{Name: "I", BowlScore: 0.7, IsBowler: true},
		{Name: "J", BowlScore: 0.6, IsBowler: true},
		{Name: "K", BowlScore: 0.5, IsBowler: true},
	}
}

func TestSelectOptimized_ValidPool(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()
	sel, err := ts.SelectOptimized(pool, w, ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true})
	require.NoError(t, err)
	require.Len(t, sel, 11)
	bowlers := 0
	keepers := 0
	for _, p := range sel {
		if p.IsBowler {
			bowlers++
		}
		if p.IsKeeper {
			keepers++
		}
	}
	require.GreaterOrEqual(t, bowlers, 5, "at least 5 bowlers")
	require.GreaterOrEqual(t, keepers, 1, "at least 1 keeper")
}

func TestSelectOptimized_Errors(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()

	testCases := []struct {
		name   string
		pool   []ts.Player
		c      ts.Constraints
		errStr string
	}{
		{"invalid size", pool, ts.Constraints{Size: 0, MinBowlers: 5}, "invalid size"},
		{"insufficient pool", pool[:5], ts.Constraints{Size: 11, MinBowlers: 2}, "insufficient pool"},
		// Pool of 11 with no keeper
		{"no keeper", func() []ts.Player {
			p := mkPool()
			p[0].IsKeeper = false
			return p
		}(), ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true}, "no keeper"},
		{"not enough bowlers", pool[:8], ts.Constraints{Size: 8, MinBowlers: 6}, "not enough bowlers"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			_, err := ts.SelectOptimized(tc.pool, w, tc.c)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errStr)
		})
	}
}

func TestSelectTopK_Valid(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()
	xis, err := ts.SelectTopK(pool, w, ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true}, 3)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(xis), 1)
	require.LessOrEqual(t, len(xis), 3)
	for _, xi := range xis {
		require.Len(t, xi, 11, "each XI should have 11 players")
	}
}

func TestSelectTopK_InvalidK(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()
	_, err := ts.SelectTopK(pool, w, ts.Constraints{Size: 11, MinBowlers: 5}, 0)
	require.Error(t, err, "expected error for k=0")
}

func TestSelectByWinProbability_UsesEvalFunc(t *testing.T) {
	t.Parallel()
	pool := []ts.Player{
		{Name: "A", BatScore: 0.2, IsBowler: true, IsKeeper: true},
		{Name: "B", BatScore: 0.8, IsBowler: true},
		{Name: "C", BatScore: 0.6, IsBowler: true},
	}
	w := ts.DefaultWeights()
	c := ts.Constraints{Size: 2, MinBowlers: 1, RequireKeeper: true}

	evalFunc := func(names []string) (float64, error) {
		for _, n := range names {
			if n == "B" {
				return 0.9, nil
			}
		}
		return 0.1, nil
	}

	sel, err := ts.SelectByWinProbability(pool, w, c, evalFunc)
	require.NoError(t, err)
	require.Len(t, sel, 2)
	hasB := false
	for _, p := range sel {
		if p.Name == "B" {
			hasB = true
		}
	}
	require.True(t, hasB, "expected player B (high win prob) in selected XI: %v", sel)
}

func TestSelectByWinProbability_Errors(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()
	noop := func(_ []string) (float64, error) { return 0.5, nil }

	testCases := []struct {
		name   string
		pool   []ts.Player
		c      ts.Constraints
		errStr string
	}{
		{"invalid size", pool, ts.Constraints{Size: 0, MinBowlers: 5}, "invalid size"},
		{"insufficient pool", pool[:3], ts.Constraints{Size: 11, MinBowlers: 2}, "insufficient pool"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			_, err := ts.SelectByWinProbability(tc.pool, w, tc.c, noop)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errStr)
		})
	}
}

func TestSelectByWinProbability_FallsBackOnEvalError(t *testing.T) {
	t.Parallel()
	pool := []ts.Player{
		{Name: "A", BatScore: 0.9, IsBowler: true, IsKeeper: true},
		{Name: "B", BatScore: 0.5, IsBowler: true},
		{Name: "C", BatScore: 0.3, IsBowler: true},
	}
	w := ts.DefaultWeights()
	c := ts.Constraints{Size: 2, MinBowlers: 1, RequireKeeper: true}

	evalFunc := func(_ []string) (float64, error) {
		return 0, errors.New("eval error")
	}
	// Should not panic; falls back to greedy seed
	sel, err := ts.SelectByWinProbability(pool, w, c, evalFunc)
	require.NoError(t, err)
	require.Len(t, sel, 2)
}
