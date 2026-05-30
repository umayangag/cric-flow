package predictor_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/umayangag/cric-flow/go-app/internal/predictor"
	"github.com/umayangag/cric-flow/go-app/internal/predictor/internal/mocks"

	"github.com/stretchr/testify/require"
)

func TestBuildTeam_UsesPredictorAndSelectsTopDeterministically(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.2},
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	m := mocks.NewMockPredictor(t)
	m.On("PredictWin", mock.Anything, players).Return(players, nil)

	got, err := predictor.BuildTeam(ctx, m, players, 2)
	require.NoError(t, err)
	want := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	require.Equal(t, want, got)
}

func TestBuildTeam_ErrorFromPredictor(t *testing.T) {
	ctx := context.Background()
	m := mocks.NewMockPredictor(t)
	m.On("PredictWin", mock.Anything, mock.Anything).Return(nil, assertErr{})
	_, err := predictor.BuildTeam(ctx, m, nil, 11)
	require.Error(t, err)
}

func TestBuildTeam_ZeroTeamSizeReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
	}
	m := mocks.NewMockPredictor(t)
	m.On("PredictWin", mock.Anything, players).Return(players, nil)
	got, err := predictor.BuildTeam(ctx, m, players, 0)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestBuildTeam_EmptyPlayersReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{}
	m := mocks.NewMockPredictor(t)
	m.On("PredictWin", mock.Anything, players).Return(players, nil)
	got, err := predictor.BuildTeam(ctx, m, players, 11)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestBuildTeam_TeamSizeGreaterThanPlayersReturnsAllSorted(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.6},
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.6},
	}
	m := mocks.NewMockPredictor(t)
	m.On("PredictWin", mock.Anything, players).Return(players, nil)
	got, err := predictor.BuildTeam(ctx, m, players, 10)
	require.NoError(t, err)
	want := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
		// For ties (0.6) names ascending: B then C
		{PlayerName: "B", WinningProbability: 0.6},
		{PlayerName: "C", WinningProbability: 0.6},
	}
	require.Equal(t, want, got)
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }

func TestBuildTeam_TeamSizeLessThanPlayersSelectsTopN(t *testing.T) {
	ctx := context.Background()
	players := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.7},
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.6},
		{PlayerName: "D", WinningProbability: 0.8},
	}
	m := mocks.NewMockPredictor(t)
	m.On("PredictWin", mock.Anything, players).Return(players, nil)
	got, err := predictor.BuildTeam(ctx, m, players, 3)
	require.NoError(t, err)
	want := []predictor.PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "D", WinningProbability: 0.8},
		{PlayerName: "A", WinningProbability: 0.7},
	}
	require.Equal(t, want, got)
}
