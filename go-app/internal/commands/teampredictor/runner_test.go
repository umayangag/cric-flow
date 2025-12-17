package teampredictor_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teampredictor"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teampredictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teampredictor/internal/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

func TestRunner_Run_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func(ctx context.Context, mp *mocks.MockPredictor)
	type assertFn func(t *testing.T, out mlclient.PredictResponse, err error, mp *mocks.MockPredictor)

	cases := []struct {
		name    string
		opts    cli.Options
		arrange arrangeFn
		assert  assertFn
		// for special cases where we don't use a mock-backed runner
		customRun func(ctx context.Context) (mlclient.PredictResponse, error)
	}{
		{
			name: "nil runner",
			opts: cli.Options{MatchID: 1, Format: "T20", Season: "2019"},
			customRun: func(ctx context.Context) (mlclient.PredictResponse, error) {
				var rnil *cmd.Runner
				return rnil.Run(ctx, cli.Options{MatchID: 1, Format: "T20", Season: "2019"})
			},
			assert: func(t *testing.T, _ mlclient.PredictResponse, err error, _ *mocks.MockPredictor) {
				require.Error(t, err)
				require.ErrorContains(t, err, "nil runner")
			},
		},
		{
			name: "missing service",
			opts: cli.Options{MatchID: 1, Format: "T20", Season: "2019"},
			customRun: func(ctx context.Context) (mlclient.PredictResponse, error) {
				r := cmd.NewRunner(nil)
				return r.Run(ctx, cli.Options{MatchID: 1, Format: "T20", Season: "2019"})
			},
			assert: func(t *testing.T, _ mlclient.PredictResponse, err error, _ *mocks.MockPredictor) {
				require.Error(t, err)
				require.ErrorContains(t, err, "missing service")
			},
		},
		{
			name: "invalid opts short-circuits before Predict",
			opts: cli.Options{MatchID: 0, Format: "T20", Season: "2019"},
			arrange: func(_ context.Context, _ *mocks.MockPredictor) {
				// No expectations — should not call Predict
			},
			assert: func(t *testing.T, _ mlclient.PredictResponse, err error, _ *mocks.MockPredictor) {
				require.Error(t, err)
				require.ErrorContains(t, err, "invalid options")
			},
		},
		{
			name: "happy path delegates to predictor",
			opts: cli.Options{MatchID: 1, Format: "T20", Season: "2019"},
			arrange: func(ctx context.Context, mp *mocks.MockPredictor) {
				mp.EXPECT().Predict(ctx, cli.Options{MatchID: 1, Format: "T20", Season: "2019"}).
					Return(mlclient.PredictResponse{Players: []string{"X"}}, nil)
			},
			assert: func(t *testing.T, out mlclient.PredictResponse, err error, _ *mocks.MockPredictor) {
				require.NoError(t, err)
				require.Equal(t, []string{"X"}, out.Players)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// For custom-run cases (nil runner / missing service), bypass mock wiring
			if tc.customRun != nil {
				out, err := tc.customRun(ctx)
				tc.assert(t, out, err, nil)
				return
			}

			// Arrange
			mp := mocks.NewMockPredictor(t)
			if tc.arrange != nil {
				tc.arrange(ctx, mp)
			}

			r := cmd.NewRunner(mp)

			// Act
			out, err := r.Run(ctx, tc.opts)

			// Assert
			tc.assert(t, out, err, mp)
		})
	}
}
