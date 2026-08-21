package eval_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umayangag/cric-flow/go-app/internal/eval"
)

func TestMAE(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		yTrue    []float64
		yPred    []float64
		expected float64
		wantNaN  bool
	}{
		{name: "empty returns NaN", yTrue: nil, yPred: nil, wantNaN: true},
		{name: "length mismatch returns NaN", yTrue: []float64{1, 2}, yPred: []float64{1}, wantNaN: true},
		{name: "zero error", yTrue: []float64{1, 2, 3}, yPred: []float64{1, 2, 3}, expected: 0},
		{
			name:     "typical values",
			yTrue:    []float64{1, 2, 3},
			yPred:    []float64{1.5, 1.0, 2.0},
			expected: (0.5 + 1.0 + 1.0) / 3.0,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := eval.MAE(tc.yTrue, tc.yPred)

			// Assert
			if tc.wantNaN {
				assert.True(t, math.IsNaN(got), "expected NaN, got %v", got)
				return
			}
			assert.InDelta(t, tc.expected, got, 1e-12)
		})
	}
}

func TestRMSE(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		yTrue    []float64
		yPred    []float64
		expected float64
		wantNaN  bool
	}{
		{name: "empty returns NaN", yTrue: nil, yPred: nil, wantNaN: true},
		{name: "length mismatch returns NaN", yTrue: []float64{1, 2}, yPred: []float64{1}, wantNaN: true},
		{name: "zero error", yTrue: []float64{1, 2, 3}, yPred: []float64{1, 2, 3}, expected: 0},
		{
			name:     "typical values",
			yTrue:    []float64{1, 2, 3},
			yPred:    []float64{1.5, 1.0, 2.0},
			expected: math.Sqrt(((0.5 * 0.5) + (1.0 * 1.0) + (1.0 * 1.0)) / 3.0),
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := eval.RMSE(tc.yTrue, tc.yPred)

			// Assert
			if tc.wantNaN {
				assert.True(t, math.IsNaN(got), "expected NaN, got %v", got)
				return
			}
			assert.InDelta(t, tc.expected, got, 1e-12)
		})
	}
}

func TestBrierScore(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		yTrue    []float64
		yProb    []float64
		expected float64
		wantNaN  bool
	}{
		{name: "empty returns NaN", yTrue: nil, yProb: nil, wantNaN: true},
		{name: "length mismatch returns NaN", yTrue: []float64{1, 0}, yProb: []float64{0.2}, wantNaN: true},
		{name: "perfect probabilities", yTrue: []float64{1, 0, 1}, yProb: []float64{1, 0, 1}, expected: 0},
		{
			name:     "typical values",
			yTrue:    []float64{1, 0, 1, 0},
			yProb:    []float64{0.8, 0.3, 0.6, 0.1},
			expected: ((0.2 * 0.2) + (0.3 * 0.3) + (0.4 * 0.4) + (0.1 * 0.1)) / 4.0,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := eval.BrierScore(tc.yTrue, tc.yProb)

			// Assert
			if tc.wantNaN {
				assert.True(t, math.IsNaN(got), "expected NaN, got %v", got)
				return
			}
			assert.InDelta(t, tc.expected, got, 1e-12)
		})
	}
}
