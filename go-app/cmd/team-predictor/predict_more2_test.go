package main

import (
	"context"
	"reflect"
	"testing"

	fakeML "github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient/fake"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestBuildTeam_TeamSizeGreaterThanPlayersReturnsAllSorted(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.6},
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.6},
	}
	fake := &fakeML.Client{Responses: players}
	got, err := buildTeam(ctx, fake, players, 10)
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
