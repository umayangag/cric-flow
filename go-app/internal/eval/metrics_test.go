package eval

import (
	"math"
	"testing"
)

func almostEqual(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestMAE(t *testing.T) {
	tests := []struct {
		name    string
		y       []float64
		yhat    []float64
		want    float64
		wantNaN bool
	}{
		{name: "empty -> NaN", y: nil, yhat: nil, wantNaN: true},
		{name: "len mismatch -> NaN", y: []float64{1, 2}, yhat: []float64{1}, wantNaN: true},
		{name: "zero error", y: []float64{1, 2, 3}, yhat: []float64{1, 2, 3}, want: 0},
		{name: "typical", y: []float64{1, 2, 3}, yhat: []float64{1.5, 1.0, 2.0}, want: (0.5 + 1.0 + 1.0) / 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MAE(tt.y, tt.yhat)
			if tt.wantNaN {
				if !math.IsNaN(got) {
					t.Fatalf("expected NaN, got %v", got)
				}
				return
			}
			if !almostEqual(got, tt.want, 1e-12) {
				t.Fatalf("MAE got %v want %v", got, tt.want)
			}
		})
	}
}

func TestRMSE(t *testing.T) {
	tests := []struct {
		name    string
		y       []float64
		yhat    []float64
		want    float64
		wantNaN bool
	}{
		{name: "empty -> NaN", y: nil, yhat: nil, wantNaN: true},
		{name: "len mismatch -> NaN", y: []float64{1, 2}, yhat: []float64{1}, wantNaN: true},
		{name: "zero error", y: []float64{1, 2, 3}, yhat: []float64{1, 2, 3}, want: 0},
		{
			name: "typical",
			y:    []float64{1, 2, 3},
			yhat: []float64{1.5, 1.0, 2.0},
			want: math.Sqrt(((0.5 * 0.5) + (1.0 * 1.0) + (1.0 * 1.0)) / 3.0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RMSE(tt.y, tt.yhat)
			if tt.wantNaN {
				if !math.IsNaN(got) {
					t.Fatalf("expected NaN, got %v", got)
				}
				return
			}
			if !almostEqual(got, tt.want, 1e-12) {
				t.Fatalf("RMSE got %v want %v", got, tt.want)
			}
		})
	}
}

func TestBrierScore(t *testing.T) {
	tests := []struct {
		name    string
		y       []float64
		p       []float64
		want    float64
		wantNaN bool
	}{
		{name: "empty -> NaN", y: nil, p: nil, wantNaN: true},
		{name: "len mismatch -> NaN", y: []float64{1, 0}, p: []float64{0.2}, wantNaN: true},
		{name: "perfect probs", y: []float64{1, 0, 1}, p: []float64{1, 0, 1}, want: 0},
		{
			name: "typical",
			y:    []float64{1, 0, 1, 0},
			p:    []float64{0.8, 0.3, 0.6, 0.1},
			want: ((0.2 * 0.2) + (0.3 * 0.3) + (0.4 * 0.4) + (0.1 * 0.1)) / 4.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BrierScore(tt.y, tt.p)
			if tt.wantNaN {
				if !math.IsNaN(got) {
					t.Fatalf("expected NaN, got %v", got)
				}
				return
			}
			if !almostEqual(got, tt.want, 1e-12) {
				t.Fatalf("BrierScore got %v want %v", got, tt.want)
			}
		})
	}
}
