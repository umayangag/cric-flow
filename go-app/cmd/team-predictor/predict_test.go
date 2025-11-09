package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/mock"
	mlmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestBuildTeam_UsesPredictorAndSelectsTopDeterministically(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.2},
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	m := &mlmocks.Predictor{}
	m.On("PredictWin", mock.Anything, players).Return(players, nil)

	got, err := buildTeam(ctx, m, players, 2)
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
	m := &mlmocks.Predictor{}
	m.On("PredictWin", mock.Anything, mock.Anything).Return(nil, assertErr{})
	_, err := buildTeam(ctx, m, nil, 11)
	if err == nil {
		t.Fatalf("expected error from predictor")
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }
