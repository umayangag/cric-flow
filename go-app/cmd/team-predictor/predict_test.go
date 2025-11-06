package main

import (
	"context"
	"reflect"
	"testing"

	fakeML "github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient/fake"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestBuildTeam_UsesPredictorAndSelectsTopDeterministically(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.2},
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	// Fake will echo Responses; ensure deterministic selection/top-N
	fake := &fakeML.Client{Responses: players}

	got, err := buildTeam(ctx, fake, players, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("team mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestBuildTeam_ErrorFromPredictor(t *testing.T) {
	ctx := context.Background()
	fake := &fakeML.Client{Err: assertErr{}}
	_, err := buildTeam(ctx, fake, nil, 11)
	if err == nil {
		t.Fatalf("expected error from predictor")
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }
