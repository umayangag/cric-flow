package teamselect_test

import (
	"math"
	"testing"

	ts "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
)

type assertFn func(t *testing.T, got float64)

func assertFloatNear(want float64) assertFn {
	return func(t *testing.T, got float64) {
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("want %.6f got %.6f", want, got)
		}
	}
}

func TestScorePlayer_Table(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	cases := []struct {
		name   string
		p      ts.Player
		w      ts.ScoreWeights
		assert assertFn
	}{
		{
			name:   "basic mix no keeper",
			p:      ts.Player{BatScore: 0.8, BowlScore: 0.2},
			w:      w,
			assert: assertFloatNear(w.Bat*0.8 + w.Bowl*0.2),
		},
		{
			name:   "keeper bonus applied",
			p:      ts.Player{BatScore: 0.4, BowlScore: 0.6, IsKeeper: true},
			w:      w,
			assert: assertFloatNear(w.Bat*0.4 + w.Bowl*0.6 + w.KeeperBonus),
		},
		{
			name:   "clamp below 0 to 0",
			p:      ts.Player{BatScore: -1, BowlScore: 0},
			w:      w,
			assert: assertFloatNear(w.Bat*0 + w.Bowl*0),
		},
		{
			name:   "clamp above 1 to 1",
			p:      ts.Player{BatScore: 2, BowlScore: 3},
			w:      w,
			assert: assertFloatNear(w.Bat*1 + w.Bowl*1),
		},
		{
			name:   "custom weights emphasize bowling",
			p:      ts.Player{BatScore: 0.5, BowlScore: 0.9},
			w:      ts.ScoreWeights{Bat: 0.2, Bowl: 0.8, KeeperBonus: 0.01},
			assert: assertFloatNear(0.2*0.5 + 0.8*0.9),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ts.ScorePlayer(tc.p, tc.w)
			// no ifs here; delegate to assert function
			tc.assert(t, got)
		})
	}
}
