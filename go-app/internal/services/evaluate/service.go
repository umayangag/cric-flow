package evaluate

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/eval"
)

// Inputs represent time-aligned observations for error metrics.
// All slices should be the same length for valid metrics.
type Inputs struct {
	YTrue []float64 // actual values (e.g., runs)
	YPred []float64 // predicted values
	YWin  []float64 // actual outcomes in {0,1}
	YProb []float64 // predicted probability of win
}

// Metrics computed from Inputs.
type Metrics struct {
	MAE   float64
	RMSE  float64
	Brier float64
}

// EvaluationRepo defines a minimal dependency to supply evaluation inputs.
// Note: mock generation may be managed centrally; this tag is provided for future use.
//
//go:generate mockery --name EvaluationRepo --filename evaluation_repo.go --output ../../mocks --case underscore
type EvaluationRepo interface {
	LoadInputs(ctx context.Context, season, format string) (Inputs, error)
}

// ComputeMetrics computes error metrics purely from Inputs.
func ComputeMetrics(in Inputs) Metrics {
	return Metrics{
		MAE:   eval.MAE(in.YTrue, in.YPred),
		RMSE:  eval.RMSE(in.YTrue, in.YPred),
		Brier: eval.BrierScore(in.YWin, in.YProb),
	}
}
