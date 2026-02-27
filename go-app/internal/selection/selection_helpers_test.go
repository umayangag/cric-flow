package selection

import (
	"math"
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/predictor"
)

func TestSelectTopWithMinBowlers(t *testing.T) {
	preds := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.90, Deliveries: 0, Econ: 0},    // batter
		{PlayerName: "B", WinningProbability: 0.85, Deliveries: 0, Econ: 0},    // batter
		{PlayerName: "C", WinningProbability: 0.80, Deliveries: 24, Econ: 6.5}, // bowler
		{PlayerName: "D", WinningProbability: 0.70, Deliveries: 0, Econ: 0},    // batter
		{PlayerName: "E", WinningProbability: 0.65, Deliveries: 30, Econ: 7.0}, // bowler
	}

	// Case: teamSize 3, need at least 1 bowler -> should be satisfied by initial top-3 after sorting
	sel, err := selectTopWithMinBowlers(append([]predictor.PlayerPrediction{}, preds...), 3, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sel) != 3 {
		t.Fatalf("expected 3 selected, got %d", len(sel))
	}
	bowlers := 0
	for _, p := range sel {
		if p.Deliveries > 0 || p.Econ > 0 {
			bowlers++
		}
	}
	if bowlers < 1 {
		t.Fatalf("expected at least 1 bowler, got %d", bowlers)
	}

	// Case: teamSize 3, require 2 bowlers -> should swap one batter with next bowler
	sel2, err := selectTopWithMinBowlers(append([]predictor.PlayerPrediction{}, preds...), 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sel2) != 3 {
		t.Fatalf("expected 3 selected, got %d", len(sel2))
	}
	bowlers = 0
	for _, p := range sel2 {
		if p.Deliveries > 0 || p.Econ > 0 {
			bowlers++
		}
	}
	if bowlers < 2 {
		t.Fatalf("expected at least 2 bowlers after swap, got %d", bowlers)
	}

	// Case: pool too small
	_, err = selectTopWithMinBowlers(append([]predictor.PlayerPrediction{}, preds...), 10, 1)
	if err == nil {
		t.Fatalf("expected error for small pool, got nil")
	}
}

func TestComputeAverageWinProbability(t *testing.T) {
	// Empty slice -> 0
	if got := computeAverageWinProbability(nil); got != 0 {
		t.Fatalf("avg empty got %v want 0", got)
	}
	ps := []predictor.PlayerPrediction{
		{WinningProbability: 0.5},
		{WinningProbability: 0.7},
		{WinningProbability: 0.9},
	}
	want := (0.5 + 0.7 + 0.9) / 3.0
	if got := computeAverageWinProbability(ps); math.Abs(got-want) > 1e-9 {
		t.Fatalf("avg got %v want ~%v", got, want)
	}
}

func TestParseF64(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"", 0},
		{"3.14", 3.14},
		{"0", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		if got := parseF64(tt.in); got != tt.want {
			t.Errorf("parseF64(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPrevSeasonName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2024", "2023"},
		{"2019", "2018"},
		{"invalid", "invalid"},
	}
	for _, tt := range tests {
		if got := prevSeasonName(tt.in); got != tt.want {
			t.Errorf("prevSeasonName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseSeasonInt(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"2024", 2024},
		{" 2019 ", 2019},
		{"x", 0},
	}
	for _, tt := range tests {
		if got := parseSeasonInt(tt.in); got != tt.want {
			t.Errorf("parseSeasonInt(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestNz64(t *testing.T) {
	if got := nz64(struct{ Int64 int64; Valid bool }{10, true}); got != 10 {
		t.Errorf("nz64(valid) = %d, want 10", got)
	}
	if got := nz64(struct{ Int64 int64; Valid bool }{10, false}); got != 0 {
		t.Errorf("nz64(invalid) = %d, want 0", got)
	}
}

func TestF32(t *testing.T) {
	if got := f32(3.14); got != 3.14 {
		t.Errorf("f32(3.14) = %v, want 3.14", got)
	}
}
