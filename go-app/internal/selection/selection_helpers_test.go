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
