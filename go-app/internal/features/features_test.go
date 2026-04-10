package features

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func feq(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestEWM(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(vals ...float64) []Innings {
		out := make([]Innings, 0, len(vals))
		for i, v := range vals {
			out = append(out, Innings{Date: base.Add(time.Duration(i) * time.Hour), Value: v})
		}
		return out
	}
	tests := []struct {
		name  string
		inn   []Innings
		alpha float64
		wantV float64
		wantW float64
	}{
		{name: "empty", inn: nil, alpha: 0.5, wantV: 0, wantW: 0},
		{name: "alpha<=0 uses default", inn: mk(10, 20), alpha: 0.0, wantV: 15.882352941176471, wantW: 0.51},
		// For alpha 0.3 with values [10,20], stable incremental EWM yields ~16.153846, weight sum ~0.51
		{name: "typical", inn: mk(10, 20, 30), alpha: 0.5, wantV: 24.285714285714285, wantW: 0.875},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, w := EWM(tt.inn, tt.alpha)
			if !feq(v, tt.wantV, 1e-6) {
				t.Fatalf("EWM value got %v want %v", v, tt.wantV)
			}
			if !feq(w, tt.wantW, 1e-6) {
				t.Fatalf("EWM weight got %v want %v", w, tt.wantW)
			}
		})
	}
}

func TestConsistency(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(vals ...float64) []Innings {
		out := make([]Innings, 0, len(vals))
		for i, v := range vals {
			out = append(out, Innings{Date: base.Add(time.Duration(i) * time.Hour), Value: v})
		}
		return out
	}
	tests := []struct {
		name string
		inn  []Innings
		n    int
		// Expected values calculated: for [10,20,30], mean=20, std=sqrt(((10^2)+(0^2)+(10^2))/3)=sqrt(200/3)
		wantCV float64
		wantN  int
	}{
		{name: "empty", inn: nil, n: 5, wantCV: 0, wantN: 0},
		{name: "use last n window", inn: mk(10, 20, 30, 40), n: 2, wantCV: func() float64 { // values [30,40]
			mean := 35.0
			d1 := 30.0 - mean
			d2 := 40.0 - mean
			std := math.Sqrt((d1*d1 + d2*d2) / 2.0)
			return std / mean
		}(), wantN: 2},
		{name: "all values when n=0", inn: mk(10, 20, 30), n: 0, wantCV: func() float64 {
			mean := 20.0
			d1 := 10.0 - mean
			d2 := 20.0 - mean
			d3 := 30.0 - mean
			std := math.Sqrt((d1*d1 + d2*d2 + d3*d3) / 3.0)
			return std / mean
		}(), wantN: 3},
		{name: "zero mean fallback", inn: mk(0, 0, 0), n: 0, wantCV: 1, wantN: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv, n := Consistency(tt.inn, tt.n)
			if !feq(cv, tt.wantCV, 1e-9) || n != tt.wantN {
				t.Fatalf("Consistency got (cv=%v,n=%d) want (cv=%v,n=%d)", cv, n, tt.wantCV, tt.wantN)
			}
		})
	}
}

func TestMomentum(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(vals ...float64) []Innings {
		out := make([]Innings, 0, len(vals))
		for i, v := range vals {
			out = append(out, Innings{Date: base.Add(time.Duration(i) * time.Hour), Value: v})
		}
		return out
	}
	tests := []struct {
		name      string
		inn       []Innings
		n         int
		wantSlope float64
		wantN     int
	}{
		{name: "empty", inn: nil, n: 5, wantSlope: 0, wantN: 0},
		{name: "single value", inn: mk(10), n: 5, wantSlope: 0, wantN: 1},
		{name: "two values", inn: mk(10, 20), n: 0, wantSlope: 10, wantN: 2},
		{name: "last n window", inn: mk(5, 10, 15, 20, 25), n: 3, wantSlope: 5, wantN: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slope, n := Momentum(tt.inn, tt.n)
			if !feq(slope, tt.wantSlope, 1e-9) || n != tt.wantN {
				t.Fatalf("Momentum got (slope=%v,n=%d) want (slope=%v,n=%d)", slope, n, tt.wantSlope, tt.wantN)
			}
		})
	}
}

func TestSortAndClip_EmptyInput(t *testing.T) {
	cut := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	out := SortAndClip(nil, cut)
	if len(out) != 0 {
		t.Fatalf("SortAndClip(nil) should return empty, got len=%d", len(out))
	}
	out = SortAndClip([]Innings{}, cut)
	if len(out) != 0 {
		t.Fatalf("SortAndClip(empty) should return empty, got len=%d", len(out))
	}
}

func TestSortAndClip(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	inn := []Innings{
		{Date: base.Add(2 * time.Hour), Value: 3},
		{Date: base.Add(1 * time.Hour), Value: 2},
		{Date: base.Add(3 * time.Hour), Value: 4},
	}
	cut := base.Add(3 * time.Hour) // should keep first two, drop last (since not strictly before cutoff)
	out := SortAndClip(inn, cut)
	if len(out) != 2 {
		t.Fatalf("expected len 2, got %d", len(out))
	}
	if !(out[0].Date.Before(out[1].Date)) {
		t.Fatalf("expected sorted ascending by date")
	}
	// Boundary: entries on cutoff time should be excluded
	inn2 := []Innings{{Date: cut, Value: 10}}
	out2 := SortAndClip(inn2, cut)
	if len(out2) != 0 {
		t.Fatalf("expected 0 due to strict before cutoff, got %d", len(out2))
	}
}

func TestWindowedStats(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	asOf := base.Add(24 * time.Hour)
	mk := func(vals ...float64) []Innings {
		out := make([]Innings, 0, len(vals))
		for i, v := range vals {
			out = append(out, Innings{Date: base.Add(time.Duration(i) * 24 * time.Hour), Value: v})
		}
		return out
	}
	t.Run("empty", func(t *testing.T) {
		r := WindowedStats(nil, asOf)
		if r.CareerCount != 0 || r.Last1 != 0 || r.MeanW5 != 0 {
			t.Fatalf("empty innings should yield zero RawStats: %+v", r)
		}
	})
	t.Run("typical", func(t *testing.T) {
		inn := mk(10, 20, 30, 40, 50)        // last=50, last2=40, last3=30; mean_w5=30, career_mean=30
		asOf := base.Add(5 * 24 * time.Hour) // after last inning so DaysSinceLast is positive
		r := WindowedStats(inn, asOf)
		if r.CareerCount != 5 {
			t.Fatalf("CareerCount want 5 got %d", r.CareerCount)
		}
		if !feq(r.Last1, 50, 1e-9) || !feq(r.Last2, 40, 1e-9) || !feq(r.Last3, 30, 1e-9) {
			t.Fatalf("Last1/2/3 got %v %v %v", r.Last1, r.Last2, r.Last3)
		}
		if !feq(r.CareerMean, 30, 1e-9) {
			t.Fatalf("CareerMean want 30 got %v", r.CareerMean)
		}
		if !feq(r.MeanW5, 30, 1e-9) {
			t.Fatalf("MeanW5 want 30 got %v", r.MeanW5)
		}
		// trend_w5 = (50-10)/4 = 10
		if !feq(r.TrendW5, 10, 1e-9) {
			t.Fatalf("TrendW5 want 10 got %v", r.TrendW5)
		}
		// StdW5, StdW10: population variance of [10,20,30,40,50] = 200, std = sqrt(200)
		if !feq(r.StdW5, math.Sqrt(200), 1e-9) || !feq(r.StdW10, math.Sqrt(200), 1e-9) {
			t.Fatalf("StdW5/StdW10 got %v %v", r.StdW5, r.StdW10)
		}
		if !feq(r.MinW10, 10, 1e-9) || !feq(r.MaxW10, 50, 1e-9) || !feq(r.MedianW10, 30, 1e-9) {
			t.Fatalf("MinW10/MaxW10/MedianW10 got %v %v %v", r.MinW10, r.MaxW10, r.MedianW10)
		}
		if !feq(r.PctZeroW10, 0, 1e-9) {
			t.Fatalf("PctZeroW10 want 0 got %v", r.PctZeroW10)
		}
		// asOf is base+5d, last inning is base+4d; DaysSinceLast = 1
		if !feq(r.DaysSinceLast, 1, 1e-9) {
			t.Fatalf("DaysSinceLast want 1 got %v", r.DaysSinceLast)
		}
		if r.InningsInLast90D != 5 {
			t.Fatalf("InningsInLast90D want 5 got %d", r.InningsInLast90D)
		}
	})
	t.Run("n_less_than_3", func(t *testing.T) {
		inn := mk(10, 20)
		r := WindowedStats(inn, asOf)
		if r.CareerCount != 2 || !feq(r.Last1, 20, 1e-9) || !feq(r.Last2, 10, 1e-9) || r.Last3 != 0 {
			t.Fatalf("n=2: CareerCount=2 Last1=20 Last2=10 Last3=0 got %+v", r)
		}
		// Window of 2: MeanW3=15, StdW5=StdW10=std(10,20)=5
		if !feq(r.MeanW3, 15, 1e-9) || !feq(r.StdW5, 5, 1e-9) || !feq(r.StdW10, 5, 1e-9) {
			t.Fatalf("n=2: MeanW3=15 StdW5=StdW10=5 got MeanW3=%v StdW5=%v StdW10=%v", r.MeanW3, r.StdW5, r.StdW10)
		}
	})
	t.Run("n_less_than_5", func(t *testing.T) {
		inn := mk(5, 15, 25, 35)
		r := WindowedStats(inn, asOf)
		if r.CareerCount != 4 || !feq(r.MeanW5, 20, 1e-9) {
			t.Fatalf("n=4: CareerCount=4 MeanW5=20 got %d %v", r.CareerCount, r.MeanW5)
		}
		// MeanW10 = mean of all 4 = 20; StdW10 = std(5,15,25,35) = sqrt(125)
		if !feq(r.MeanW10, 20, 1e-9) || !feq(r.StdW10, math.Sqrt(125), 1e-9) {
			t.Fatalf("n=4: MeanW10=20 StdW10=sqrt(125) got %v %v", r.MeanW10, r.StdW10)
		}
	})
	t.Run("n_less_than_10_with_zeros", func(t *testing.T) {
		inn := mk(0, 10, 0, 20, 30)
		r := WindowedStats(inn, asOf)
		if !feq(r.PctZeroW10, 2.0/5.0, 1e-9) {
			t.Fatalf("PctZeroW10 want 0.4 got %v", r.PctZeroW10)
		}
		if !feq(r.MinW10, 0, 1e-9) || !feq(r.MaxW10, 30, 1e-9) {
			t.Fatalf("MinW10=0 MaxW10=30 got %v %v", r.MinW10, r.MaxW10)
		}
	})
}

func TestTemporalFeaturesFromUnix_Zero_ReturnsNeutral(t *testing.T) {
	ms, mc, ds, dc := TemporalFeaturesFromUnix(0)
	require.Zero(t, ms)
	require.Zero(t, mc)
	require.Zero(t, ds)
	require.Zero(t, dc)
}

func TestTemporalFeatures_ZeroTime_ReturnsNeutral(t *testing.T) {
	ms, mc, ds, dc := TemporalFeatures(time.Time{})
	require.Zero(t, ms)
	require.Zero(t, mc)
	require.Zero(t, ds)
	require.Zero(t, dc)
}
