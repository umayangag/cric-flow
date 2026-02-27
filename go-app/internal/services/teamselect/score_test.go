package teamselect_test

import (
	"math"
	"testing"

	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
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

func TestDefaultWeights(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	if w.Bat != 0.45 || w.Bowl != 0.40 || w.Field != 0.10 || w.KeeperBonus != 0.02 {
		t.Fatalf("DefaultWeights: got Bat=%.2f Bowl=%.2f Field=%.2f KeeperBonus=%.2f", w.Bat, w.Bowl, w.Field, w.KeeperBonus)
	}
}

func TestNormalizeBatScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		runs       float64
		batDivisor float64
		want       float64
	}{
		{"under cap", 50, 100, 0.5},
		{"over cap", 120, 100, 1.0},
		{"zero runs", 0, 100, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ts.NormalizeBatScore(tt.runs, tt.batDivisor)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("NormalizeBatScore(%v,%v)=%v want %v", tt.runs, tt.batDivisor, got, tt.want)
			}
		})
	}
}

func TestNormalizeBowlScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ts.NormalizeBowlScore(tt.wickets, tt.economy, tt.wicketDivisor, tt.econBase)
			wickPart := math.Min(1, tt.wickets/tt.wicketDivisor)
			econPart := math.Max(0, 1-(tt.economy/tt.econBase))
			want := (wickPart + econPart) / 2
			if math.Abs(got-want) > 1e-9 {
				t.Fatalf("NormalizeBowlScore=%v want %v", got, want)
			}
		})
	}
}

func TestNormalizeFieldScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		catches      float64
		runOuts      float64
		fieldDivisor float64
	}{
		{"under cap", 2, 1, 10},
		{"over cap", 5, 3, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ts.NormalizeFieldScore(tt.catches, tt.runOuts, tt.fieldDivisor)
			raw := (tt.catches + tt.runOuts*1.5) / tt.fieldDivisor
			want := math.Min(1, raw)
			if math.Abs(got-want) > 1e-9 {
				t.Fatalf("NormalizeFieldScore=%v want %v", got, want)
			}
		})
	}
}
