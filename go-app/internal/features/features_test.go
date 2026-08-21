package features_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/features"
)

func mkInnings(base time.Time, vals ...float64) []features.Innings {
	out := make([]features.Innings, 0, len(vals))
	for i, v := range vals {
		out = append(out, features.Innings{Date: base.Add(time.Duration(i) * time.Hour), Value: v})
	}
	return out
}

func mkInningsDaily(base time.Time, vals ...float64) []features.Innings {
	out := make([]features.Innings, 0, len(vals))
	for i, v := range vals {
		out = append(out, features.Innings{Date: base.Add(time.Duration(i) * 24 * time.Hour), Value: v})
	}
	return out
}

func TestEWM(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		inn       []features.Innings
		alpha     float64
		expectedV float64
		expectedW float64
	}{
		{name: "empty", inn: nil, alpha: 0.5, expectedV: 0, expectedW: 0},
		{
			name:      "alpha zero uses default",
			inn:       mkInnings(base, 10, 20),
			alpha:     0.0,
			expectedV: 15.882352941176471,
			expectedW: 0.51,
		},
		{
			name:      "typical three values",
			inn:       mkInnings(base, 10, 20, 30),
			alpha:     0.5,
			expectedV: 24.285714285714285,
			expectedW: 0.875,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			v, w := features.EWM(tc.inn, tc.alpha)

			// Assert
			assert.InDelta(t, tc.expectedV, v, 1e-6)
			assert.InDelta(t, tc.expectedW, w, 1e-6)
		})
	}
}

func TestConsistency(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name       string
		inn        []features.Innings
		n          int
		expectedCV float64
		expectedN  int
	}{
		{name: "empty", inn: nil, n: 5, expectedCV: 0, expectedN: 0},
		{name: "use last n window", inn: mkInnings(base, 10, 20, 30, 40), n: 2, expectedCV: func() float64 {
			mean := 35.0
			d1 := 30.0 - mean
			d2 := 40.0 - mean
			std := math.Sqrt((d1*d1 + d2*d2) / 2.0)
			return std / mean
		}(), expectedN: 2},
		{name: "all values when n is zero", inn: mkInnings(base, 10, 20, 30), n: 0, expectedCV: func() float64 {
			mean := 20.0
			d1 := 10.0 - mean
			d2 := 20.0 - mean
			d3 := 30.0 - mean
			std := math.Sqrt((d1*d1 + d2*d2 + d3*d3) / 3.0)
			return std / mean
		}(), expectedN: 3},
		{name: "zero mean fallback", inn: mkInnings(base, 0, 0, 0), n: 0, expectedCV: 1, expectedN: 3},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			cv, n := features.Consistency(tc.inn, tc.n)

			// Assert
			assert.InDelta(t, tc.expectedCV, cv, 1e-9)
			assert.Equal(t, tc.expectedN, n)
		})
	}
}

func TestMomentum(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name          string
		inn           []features.Innings
		n             int
		expectedSlope float64
		expectedN     int
	}{
		{name: "empty", inn: nil, n: 5, expectedSlope: 0, expectedN: 0},
		{name: "single value", inn: mkInnings(base, 10), n: 5, expectedSlope: 0, expectedN: 1},
		{name: "two values", inn: mkInnings(base, 10, 20), n: 0, expectedSlope: 10, expectedN: 2},
		{name: "last n window", inn: mkInnings(base, 5, 10, 15, 20, 25), n: 3, expectedSlope: 5, expectedN: 3},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			slope, n := features.Momentum(tc.inn, tc.n)

			// Assert
			assert.InDelta(t, tc.expectedSlope, slope, 1e-9)
			assert.Equal(t, tc.expectedN, n)
		})
	}
}

func TestSortAndClip(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("nil input returns empty", func(t *testing.T) {
		t.Parallel()
		cut := base.Add(12 * time.Hour)
		out := features.SortAndClip(nil, cut)
		assert.Empty(t, out)
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		t.Parallel()
		cut := base.Add(12 * time.Hour)
		out := features.SortAndClip([]features.Innings{}, cut)
		assert.Empty(t, out)
	})

	t.Run("sorts and clips before cutoff", func(t *testing.T) {
		t.Parallel()
		inn := []features.Innings{
			{Date: base.Add(2 * time.Hour), Value: 3},
			{Date: base.Add(1 * time.Hour), Value: 2},
			{Date: base.Add(3 * time.Hour), Value: 4},
		}
		cut := base.Add(3 * time.Hour)
		out := features.SortAndClip(inn, cut)
		require.Len(t, out, 2)
		assert.True(t, out[0].Date.Before(out[1].Date), "expected sorted ascending by date")
	})

	t.Run("entries on cutoff time excluded", func(t *testing.T) {
		t.Parallel()
		cut := base.Add(3 * time.Hour)
		inn := []features.Innings{{Date: cut, Value: 10}}
		out := features.SortAndClip(inn, cut)
		assert.Empty(t, out)
	})
}

func TestWindowedStats(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	asOf := base.Add(24 * time.Hour)

	t.Run("empty innings yields zero stats", func(t *testing.T) {
		t.Parallel()
		r := features.WindowedStats(nil, asOf)
		assert.Equal(t, 0, r.CareerCount)
		assert.Equal(t, 0.0, r.Last1)
		assert.Equal(t, 0.0, r.MeanW5)
	})

	t.Run("typical five values", func(t *testing.T) {
		t.Parallel()
		inn := mkInningsDaily(base, 10, 20, 30, 40, 50)
		asOfTypical := base.Add(5 * 24 * time.Hour)
		r := features.WindowedStats(inn, asOfTypical)

		assert.Equal(t, 5, r.CareerCount)
		assert.InDelta(t, 50.0, r.Last1, 1e-9)
		assert.InDelta(t, 40.0, r.Last2, 1e-9)
		assert.InDelta(t, 30.0, r.Last3, 1e-9)
		assert.InDelta(t, 30.0, r.CareerMean, 1e-9)
		assert.InDelta(t, 30.0, r.MeanW5, 1e-9)
		assert.InDelta(t, 10.0, r.TrendW5, 1e-9)
		assert.InDelta(t, math.Sqrt(200), r.StdW5, 1e-9)
		assert.InDelta(t, math.Sqrt(200), r.StdW10, 1e-9)
		assert.InDelta(t, 10.0, r.MinW10, 1e-9)
		assert.InDelta(t, 50.0, r.MaxW10, 1e-9)
		assert.InDelta(t, 30.0, r.MedianW10, 1e-9)
		assert.InDelta(t, 0.0, r.PctZeroW10, 1e-9)
		assert.InDelta(t, 1.0, r.DaysSinceLast, 1e-9)
		assert.Equal(t, 5, r.InningsInLast90D)
	})

	t.Run("n less than 3", func(t *testing.T) {
		t.Parallel()
		inn := mkInningsDaily(base, 10, 20)
		r := features.WindowedStats(inn, asOf)

		assert.Equal(t, 2, r.CareerCount)
		assert.InDelta(t, 20.0, r.Last1, 1e-9)
		assert.InDelta(t, 10.0, r.Last2, 1e-9)
		assert.Equal(t, 0.0, r.Last3)
		assert.InDelta(t, 15.0, r.MeanW3, 1e-9)
		assert.InDelta(t, 5.0, r.StdW5, 1e-9)
		assert.InDelta(t, 5.0, r.StdW10, 1e-9)
	})

	t.Run("n less than 5", func(t *testing.T) {
		t.Parallel()
		inn := mkInningsDaily(base, 5, 15, 25, 35)
		r := features.WindowedStats(inn, asOf)

		assert.Equal(t, 4, r.CareerCount)
		assert.InDelta(t, 20.0, r.MeanW5, 1e-9)
		assert.InDelta(t, 20.0, r.MeanW10, 1e-9)
		assert.InDelta(t, math.Sqrt(125), r.StdW10, 1e-9)
	})

	t.Run("n less than 10 with zeros", func(t *testing.T) {
		t.Parallel()
		inn := mkInningsDaily(base, 0, 10, 0, 20, 30)
		r := features.WindowedStats(inn, asOf)

		assert.InDelta(t, 2.0/5.0, r.PctZeroW10, 1e-9)
		assert.InDelta(t, 0.0, r.MinW10, 1e-9)
		assert.InDelta(t, 30.0, r.MaxW10, 1e-9)
	})
}
