package main

import (
	"context"
	"reflect"
	"testing"

	fakeML "github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient/fake"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestBuildTeam_TeamSizeLessThanPlayersSelectsTopN(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.7},
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.6},
		{PlayerName: "D", WinningProbability: 0.8},
	}
	fake := &fakeML.Client{Responses: players}
	got, err := buildTeam(ctx, fake, players, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "D", WinningProbability: 0.8},
		{PlayerName: "A", WinningProbability: 0.7},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected team\n got: %#v\nwant: %#v", got, want)
	}
}
