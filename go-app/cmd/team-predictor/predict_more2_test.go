package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/mock"
	mlmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestBuildTeam_TeamSizeGreaterThanPlayersReturnsAllSorted(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.6},
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.6},
	}
	m := &mlmocks.Predictor{}
	m.On("PredictWin", mock.Anything, players).Return(players, nil)
	got, err := buildTeam(ctx, m, players, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
		// For ties (0.6) names ascending: B then C
		{PlayerName: "B", WinningProbability: 0.6},
		{PlayerName: "C", WinningProbability: 0.6},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected team\n got: %#v\nwant: %#v", got, want)
	}
}
