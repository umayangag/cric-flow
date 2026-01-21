// Package eval provides evaluation metrics and helpers to compare
// predictions vs actual outcomes (player- and team-level).
package eval

import "math"

// validateSameLengthNonEmpty returns true when both input slices are non-empty
// and have the same length. All metrics in this package rely on this shape
// property; otherwise, they must return NaN to signal invalid inputs.
func validateSameLengthNonEmpty(a, b []float64) bool {
	return len(a) > 0 && len(a) == len(b)
}

// MAE computes mean absolute error.
func MAE(yTrue, yPred []float64) float64 {
	if !validateSameLengthNonEmpty(yTrue, yPred) {
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
	if !validateSameLengthNonEmpty(yTrue, yPred) {
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
	if !validateSameLengthNonEmpty(yTrue, yProb) {
		return math.NaN()
	}
	sum := 0.0
	for i := range yTrue {
		d := yTrue[i] - yProb[i]
		sum += d * d
	}
	return sum / float64(len(yTrue))
}
