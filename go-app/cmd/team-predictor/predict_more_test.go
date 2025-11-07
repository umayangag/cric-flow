package main

import (
	"context"
	"testing"

	fakeML "github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient/fake"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestBuildTeam_ZeroTeamSizeReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
	}
	fake := &fakeML.Client{Responses: players}
	got, err := buildTeam(ctx, fake, players, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty team for size 0, got %d", len(got))
	}
}

func TestBuildTeam_EmptyPlayersReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{}
	fake := &fakeML.Client{Responses: players}
	got, err := buildTeam(ctx, fake, players, 11)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty team for empty input, got %d", len(got))
	}
}
