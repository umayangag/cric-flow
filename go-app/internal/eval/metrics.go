// Package eval provides evaluation metrics and helpers to compare
// predictions vs actual outcomes (player- and team-level).
package eval

import "math"

// MAE computes mean absolute error.
func MAE(yTrue, yPred []float64) float64 {
	if len(yTrue) == 0 || len(yTrue) != len(yPred) {
		return math.NaN()
	}
	sum := 0.0
	for i := range yTrue {
		sum += math.Abs(yTrue[i] - yPred[i])
	}
	return sum / float64(len(yTrue))
}

// RMSE computes root mean squared error.
func RMSE(yTrue, yPred []float64) float64 {
	if len(yTrue) == 0 || len(yTrue) != len(yPred) {
		return math.NaN()
	}
	sum := 0.0
	for i := range yTrue {
		d := yTrue[i] - yPred[i]
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(yTrue)))
}

// BrierScore computes the mean squared error for probabilistic (0-1) predictions.
func BrierScore(yTrue, yProb []float64) float64 {
	if len(yTrue) == 0 || len(yTrue) != len(yProb) {
		return math.NaN()
	}
	sum := 0.0
	for i := range yTrue {
		d := yTrue[i] - yProb[i]
		sum += d * d
	}
	return sum / float64(len(yTrue))
}
