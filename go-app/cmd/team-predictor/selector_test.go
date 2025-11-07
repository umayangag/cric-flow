package main

import (
	"reflect"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestSelectTop_BasicAndEdgeCases(t *testing.T) {
	// zero team size returns empty
	if got := selectTop([]predictor.PlayerPrediction{{PlayerName: "A", WinningProbability: 0.9}}, 0); len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}

	// fewer than team size returns all in sorted order
	in1 := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "B", WinningProbability: 0.8},
	}
	got1 := selectTop(in1, 5)
	want1 := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "B", WinningProbability: 0.8},
	}
	if !reflect.DeepEqual(got1, want1) {
		t.Fatalf("unexpected result: got=%#v want=%#v", got1, want1)
	}

	// deterministic tie-breaker by name
	in2 := []predictor.PlayerPrediction{
		{PlayerName: "Zed", WinningProbability: 0.7},
		{PlayerName: "Ann", WinningProbability: 0.7},
	}
	got2 := selectTop(in2, 2)
	if got2[0].PlayerName != "Ann" || got2[1].PlayerName != "Zed" {
		t.Fatalf("unexpected tie order: %#v", got2)
	}

	// select top N by probability desc
	in3 := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.1},
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	got3 := selectTop(in3, 2)
	want3 := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	if !reflect.DeepEqual(got3, want3) {
		t.Fatalf("unexpected top2: got=%#v want=%#v", got3, want3)
	}

	// does not mutate input slice
	in4 := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "A", WinningProbability: 0.8},
	}
	_ = selectTop(in4, 1)
	if in4[0].PlayerName != "B" || in4[1].PlayerName != "A" {
		t.Fatalf("input mutated: %#v", in4)
	}
}
