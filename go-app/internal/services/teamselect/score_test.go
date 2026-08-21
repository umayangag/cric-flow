package teamselect_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

type assertFn func(t *testing.T, got float64)

func assertFloatNear(want float64) assertFn {
	return func(t *testing.T, got float64) {
		require.InDelta(t, want, got, 1e-9, "score")
	}
}

func TestScorePlayer_Table(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	testCases := []struct {
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
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := ts.ScorePlayer(tc.p, tc.w)
			// no ifs here; delegate to assert function
			tc.assert(t, got)
		})
	}
}

func TestDefaultWeights(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	require.Equal(t, 0.45, w.Bat)
	require.Equal(t, 0.40, w.Bowl)
	require.Equal(t, 0.10, w.Field)
	require.Equal(t, 0.02, w.KeeperBonus)
}

func TestNormalizeBatScore(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		runs       float64
		batDivisor float64
		want       float64
	}{
		{"under cap", 50, 100, 0.5},
		{"over cap", 120, 100, 1.0},
		{"zero runs", 0, 100, 0},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := ts.NormalizeBatScore(tc.runs, tc.batDivisor)
			require.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

func TestNormalizeBowlScore(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		wickets         float64
		economy         float64
		wicketDivisor   float64
		econBase        float64
		wantWickPart    float64
		wantEconPartMin float64
	}{
		{"wickets capped", 8, 7, 6, 10, 1.0, 0.3},
		{"economy at base", 3, 10, 6, 10, 0.5, 0},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := ts.NormalizeBowlScore(tc.wickets, tc.economy, tc.wicketDivisor, tc.econBase)
			wickPart := math.Min(1, tc.wickets/tc.wicketDivisor)
			econPart := math.Max(0, 1-(tc.economy/tc.econBase))
			want := (wickPart + econPart) / 2
			require.InDelta(t, want, got, 1e-9)
		})
	}
}

func TestNormalizeFieldScore(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name         string
		catches      float64
		runOuts      float64
		fieldDivisor float64
	}{
		{"under cap", 2, 1, 10},
		{"over cap", 5, 3, 4},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := ts.NormalizeFieldScore(tc.catches, tc.runOuts, tc.fieldDivisor)
			raw := (tc.catches + tc.runOuts*1.5) / tc.fieldDivisor
			want := math.Min(1, raw)
			require.InDelta(t, want, got, 1e-9)
		})
	}
}
