package evaluate_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/evaluate"
)

type assertMetricsFn func(t *testing.T, got svc.Metrics)

func assertApprox(want svc.Metrics, eps float64) assertMetricsFn {
	return func(t *testing.T, got svc.Metrics) {
		require.InDelta(t, want.MAE, got.MAE, eps, "MAE")
		require.InDelta(t, want.RMSE, got.RMSE, eps, "RMSE")
		require.InDelta(t, want.Brier, got.Brier, eps, "Brier")
	}
}

func assertNaNs() assertMetricsFn {
	return func(t *testing.T, got svc.Metrics) {
		require.True(t, math.IsNaN(got.MAE), "MAE want NaN got %v", got.MAE)
		require.True(t, math.IsNaN(got.RMSE), "RMSE want NaN got %v", got.RMSE)
		require.True(t, math.IsNaN(got.Brier), "Brier want NaN got %v", got.Brier)
	}
}

func TestComputeMetrics(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		in     svc.Inputs
		assert assertMetricsFn
	}{
		{
			name: "happy path",
			in: svc.Inputs{
				YTrue: []float64{30, 45, 10, 60},
				YPred: []float64{28, 40, 12, 55},
				YWin:  []float64{1, 0, 1, 1},
				YProb: []float64{0.7, 0.4, 0.65, 0.8},
			},
			assert: assertApprox(svc.Metrics{MAE: 3.5, RMSE: 3.807886553, Brier: 0.103125}, 1e-6),
		},
		{
			name:   "length mismatch yields NaNs",
			in:     svc.Inputs{YTrue: []float64{1, 2}, YPred: []float64{1}, YWin: []float64{1}, YProb: []float64{}},
			assert: assertNaNs(),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := svc.ComputeMetrics(tc.in)
			tc.assert(t, got)
		})
	}
}
