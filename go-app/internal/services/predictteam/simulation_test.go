package predictteam

import (
	"math/rand"
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

func TestMeanStd(t *testing.T) {
	t.Parallel()
	mean, std := meanStd([]float64{10, 20, 30})
	if mean != 20 {
		t.Errorf("mean = %v, want 20", mean)
	}
	// Population std: sqrt(((10-20)^2 + 0 + (30-20)^2)/3) = sqrt(200/3) ≈ 8.165
	if std < 8 || std > 8.2 {
		t.Errorf("std = %v, want ~8.165", std)
	}
	mean, std = meanStd(nil)
	if mean != 0 || std != 0 {
		t.Errorf("nil: mean=%v std=%v", mean, std)
	}
}

func TestPercentiles(t *testing.T) {
	t.Parallel()
	// 100 values 0..99 → P10≈9, P50≈49, P90≈89
	x := make([]float64, 100)
	for i := range x {
		x[i] = float64(i)
	}
	p10, p50, p90 := percentiles(x, 10, 50, 90)
	if p10 < 8 || p10 > 11 {
		t.Errorf("p10 = %v, want ~9", p10)
	}
	if p50 < 48 || p50 > 51 {
		t.Errorf("p50 = %v, want ~49", p50)
	}
	if p90 < 88 || p90 > 92 {
		t.Errorf("p90 = %v, want ~89", p90)
	}
}

func TestRunSimulation(t *testing.T) {
	t.Parallel()
	// Two XIs of one player each; deterministic-ish with fixed seed so totals differ
	topK1 := [][]teamselect.Player{{{Name: "A", BatScore: 0.5, BowlScore: 0, FieldScore: 0, IsBowler: false, IsKeeper: false}}}
	topK2 := [][]teamselect.Player{{{Name: "B", BatScore: 0.4, BowlScore: 0, FieldScore: 0, IsBowler: false, IsKeeper: false}}}
	nameToPred1 := map[string]PlayerPred{"A": {Runs: 25}}
	nameToPred2 := map[string]PlayerPred{"B": {Runs: 20}}
	opts := SimulationOpts{
		TopKPerTeam:          1,
		NumSamplesPerMatchup:  200,
		MaxMatchups:          1,
		RunsCV:               0.3,
		Seed:                 42,
	}
	res, err := runSimulation(topK1, topK2, nameToPred1, nameToPred2, 2, 2, "T1", "T2", opts)
	if err != nil {
		t.Fatalf("runSimulation: %v", err)
	}
	if res.NumSamples != 200 {
		t.Errorf("NumSamples = %v, want 200", res.NumSamples)
	}
	if res.NumMatchups != 1 {
		t.Errorf("NumMatchups = %v, want 1", res.NumMatchups)
	}
	sum := res.WinProbabilityTeam1 + res.WinProbabilityTeam2 + res.DrawProbability
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("win1+win2+draw = %v, want ~1", sum)
	}
	if res.Innings1TotalMean <= 0 || res.Innings2TotalMean <= 0 {
		t.Errorf("innings means should be positive: %v, %v", res.Innings1TotalMean, res.Innings2TotalMean)
	}
}

func TestSampleRuns(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		x := sampleRuns(30, 0.35, rng)
		if x < 0 {
			t.Errorf("sampleRuns(30, 0.35) = %v (negative)", x)
		}
	}
	if sampleRuns(0, 0.35, rng) != 0 {
		t.Error("sampleRuns(0) should be 0")
	}
}
