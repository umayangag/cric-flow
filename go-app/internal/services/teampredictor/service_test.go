package teampredictor_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/teampredictor"
	"github.com/umayangag/cric-flow/go-app/internal/services/teampredictor/internal/mocks"
)

type svcAssertFn func(t *testing.T, out mlclient.PredictResponse, err error)

func assertNoErrorPlayers(want []string) svcAssertFn {
	return func(t *testing.T, out mlclient.PredictResponse, err error) {
		require.NoError(t, err)
		require.Equal(t, want, out.Players)
	}
}

func assertErrContains(sub string) svcAssertFn {
	return func(t *testing.T, _ mlclient.PredictResponse, err error) {
		require.Error(t, err)
		require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(sub))
	}
}

func TestService_Predict_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func(t *testing.T) *svc.Service

	testCases := []struct {
		name    string
		opts    svc.Options
		arrange arrangeFn
		assert  svcAssertFn
	}{
		{
			name: "happy path",
			opts: svc.Options{MatchID: 1, Format: "T20", Season: "2019", Bat: 6, Bowl: 5},
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
			opts: svc.Options{MatchID: 2, Format: "ODI", Season: "2011"},
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
			opts: svc.Options{MatchID: 0, Format: "T20", Season: "2019"},
			arrange: func(t *testing.T) *svc.Service {
				// Predict should short-circuit before calling client
				m := mocks.NewMockMLClient(t)
				return svc.NewService(m)
			},
			assert: assertErrContains("invalid options"),
		},
		{
			name: "nil service",
			opts: svc.Options{MatchID: 1, Format: "T20", Season: "2019"},
			arrange: func(_ *testing.T) *svc.Service {
				return svc.NewService(nil)
			},
			assert: assertErrContains("nil service"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
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
