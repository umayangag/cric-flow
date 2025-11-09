package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	mlmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestBuildTeam_ZeroTeamSizeReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
	}
	m := &mlmocks.Predictor{}
	m.On("PredictWin", mock.Anything, players).Return(players, nil)
	got, err := buildTeam(ctx, m, players, 0)
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
	m := &mlmocks.Predictor{}
	m.On("PredictWin", mock.Anything, players).Return(players, nil)
	got, err := buildTeam(ctx, m, players, 11)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty team for empty input, got %d", len(got))
	}
}
