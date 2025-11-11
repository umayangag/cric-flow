package evaluate_test

import (
	"math"
	"testing"

	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/evaluate"
)

type assertMetricsFn func(t *testing.T, got svc.Metrics)

type assertNoErrFn func(t *testing.T, err error)

func assertApprox(want svc.Metrics, eps float64) assertMetricsFn {
	return func(t *testing.T, got svc.Metrics) {
		if !approx(got.MAE, want.MAE, eps) { t.Fatalf("MAE want %.4f got %.4f", want.MAE, got.MAE) }
		if !approx(got.RMSE, want.RMSE, eps) { t.Fatalf("RMSE want %.4f got %.4f", want.RMSE, got.RMSE) }
		if !approx(got.Brier, want.Brier, eps) { t.Fatalf("Brier want %.4f got %.4f", want.Brier, got.Brier) }
	}
}

func assertNaNs() assertMetricsFn {
	return func(t *testing.T, got svc.Metrics) {
		if !math.IsNaN(got.MAE) { t.Fatalf("MAE want NaN got %.4f", got.MAE) }
		if !math.IsNaN(got.RMSE) { t.Fatalf("RMSE want NaN got %.4f", got.RMSE) }
		if !math.IsNaN(got.Brier) { t.Fatalf("Brier want NaN got %.4f", got.Brier) }
	}
}

func approx(a, b, eps float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) { return true }
	if a > b { return a-b < eps }
	return b-a < eps
}

func TestComputeMetrics(t *testing.T) {
	t.Parallel()
	cases := []struct{
		name string
		in   svc.Inputs
		assert assertMetricsFn
	}{
		{
			name: "happy path",
			in: svc.Inputs{YTrue: []float64{30,45,10,60}, YPred: []float64{28,40,12,55}, YWin: []float64{1,0,1,1}, YProb: []float64{0.7,0.4,0.65,0.8}},
			assert: assertApprox(svc.Metrics{MAE: 3.5, RMSE: 3.807886553, Brier: 0.103125}, 1e-6),
		},
		{
			name: "length mismatch yields NaNs",
			in: svc.Inputs{YTrue: []float64{1,2}, YPred: []float64{1}, YWin: []float64{1}, YProb: []float64{}},
			assert: assertNaNs(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.ComputeMetrics(tc.in)
			tc.assert(t, got)
		})
	}
}
