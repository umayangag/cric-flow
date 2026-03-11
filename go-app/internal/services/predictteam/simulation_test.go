package predictteam

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

func TestMeanStd(t *testing.T) {
	t.Parallel()
	mean, std := meanStd([]float64{10, 20, 30})
	require.Equal(t, 20.0, mean, "mean")
	// Population std: sqrt(((10-20)^2 + 0 + (30-20)^2)/3) = sqrt(200/3) ≈ 8.165
	require.InDelta(t, 8.165, std, 0.2, "std")
	mean, std = meanStd(nil)
	require.Equal(t, 0.0, mean, "nil mean")
	require.Equal(t, 0.0, std, "nil std")
}

func TestPercentiles(t *testing.T) {
	t.Parallel()
	// 100 values 0..99 → P10≈9, P50≈49, P90≈89
	x := make([]float64, 100)
	for i := range x {
		x[i] = float64(i)
	}
	p10, p50, p90 := percentiles(x, 10, 50, 90)
	require.InDelta(t, 9, p10, 2, "p10")
	require.InDelta(t, 49, p50, 2, "p50")
	require.InDelta(t, 89, p90, 2, "p90")
}

func TestRunSimulation(t *testing.T) {
	t.Parallel()
	// Two XIs of one player each; deterministic-ish with fixed seed so totals differ
	topK1 := [][]teamselect.Player{
		{{Name: "A", BatScore: 0.5, BowlScore: 0, FieldScore: 0, IsBowler: false, IsKeeper: false}},
	}
	topK2 := [][]teamselect.Player{
		{{Name: "B", BatScore: 0.4, BowlScore: 0, FieldScore: 0, IsBowler: false, IsKeeper: false}},
	}
	nameToPred1 := map[string]PlayerPred{"A": {Runs: 25}}
	nameToPred2 := map[string]PlayerPred{"B": {Runs: 20}}
	opts := SimulationOpts{
		TopKPerTeam:          1,
		NumSamplesPerMatchup: 200,
		MaxMatchups:          1,
		RunsCV:               0.3,
		Seed:                 42,
	}
	res := runSimulation(topK1, topK2, nameToPred1, nameToPred2, 2, 2, "T1", "T2", opts)
	require.Equal(t, 200, res.NumSamples, "NumSamples")
	require.Equal(t, 1, res.NumMatchups, "NumMatchups")
	sum := res.WinProbabilityTeam1 + res.WinProbabilityTeam2 + res.DrawProbability
	require.InDelta(t, 1.0, sum, 0.01, "win1+win2+draw")
	require.Greater(t, res.Innings1TotalMean, 0.0, "innings1 mean positive")
	require.Greater(t, res.Innings2TotalMean, 0.0, "innings2 mean positive")
}

func TestSampleRuns(t *testing.T) {
	t.Parallel()
	// #nosec G404 — reproducible test
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		x := sampleRuns(30, 0.35, rng)
		require.GreaterOrEqual(t, x, 0.0, "sampleRuns(30, 0.35) non-negative")
	}
	require.Equal(t, 0.0, sampleRuns(0, 0.35, rng), "sampleRuns(0) should be 0")
}
