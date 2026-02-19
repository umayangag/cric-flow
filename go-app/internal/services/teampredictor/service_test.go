package teampredictor_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/umayangag/cric-flow/go-app/internal/cli/teampredictor"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/teampredictor"
	"github.com/umayangag/cric-flow/go-app/internal/services/teampredictor/internal/mocks"
)

type assertFn func(t *testing.T, out mlclient.PredictResponse, err error)

func assertNoErrorPlayers(want []string) assertFn {
	return func(t *testing.T, out mlclient.PredictResponse, err error) {
		require.NoError(t, err)
		require.Equal(t, want, out.Players)
	}
}

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ mlclient.PredictResponse, err error) {
		require.Error(t, err)
		require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(sub))
	}
}

func TestService_Predict_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func(t *testing.T) *svc.Service

	cases := []struct {
		name    string
		opts    cli.Options
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "happy path",
			opts: cli.Options{MatchID: 1, Format: "T20", Season: "2019", Bat: 6, Bowl: 5},
			arrange: func(t *testing.T) *svc.Service {
				m := mocks.NewMockMLClient(t)
				m.EXPECT().PredictTeam(
					context.Background(),
					mlclient.PredictRequest{MatchID: 1, Format: "T20", Season: "2019", Bat: 6, Bowl: 5},
				).Return(mlclient.PredictResponse{Players: []string{"A", "B", "C"}}, nil)
				return svc.NewService(m)
			},
			assert: assertNoErrorPlayers([]string{"A", "B", "C"}),
		},
		{
			name: "client error surfaces",
			opts: cli.Options{MatchID: 2, Format: "ODI", Season: "2011"},
			arrange: func(t *testing.T) *svc.Service {
				m := mocks.NewMockMLClient(t)
				m.EXPECT().PredictTeam(
					context.Background(),
					mlclient.PredictRequest{MatchID: 2, Format: "ODI", Season: "2011", Bat: 0, Bowl: 0},
				).Return(mlclient.PredictResponse{}, errors.New("client down"))
				return svc.NewService(m)
			},
			assert: assertErrContains("client down"),
		},
		{
			name: "invalid options",
			opts: cli.Options{MatchID: 0, Format: "T20", Season: "2019"},
			arrange: func(t *testing.T) *svc.Service {
				// Predict should short-circuit before calling client
				m := mocks.NewMockMLClient(t)
				return svc.NewService(m)
			},
			assert: assertErrContains("invalid options"),
		},
		{
			name: "nil service",
			opts: cli.Options{MatchID: 1, Format: "T20", Season: "2019"},
			arrange: func(_ *testing.T) *svc.Service {
				return svc.NewService(nil)
			},
			assert: assertErrContains("nil service"),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			s := tc.arrange(t)
			// Act
			out, err := s.Predict(context.Background(), tc.opts)
			// Assert
			tc.assert(t, out, err)
		})
	}
}
