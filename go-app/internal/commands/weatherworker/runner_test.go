package weatherworker_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherworker"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/weatherworker"
)

type assertFn func(t *testing.T, err error)

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, err error) {
		require.Error(t, err)
		require.ErrorContains(t, err, sub)
	}
}

func assertNoError() assertFn {
	return func(t *testing.T, err error) {
		require.NoError(t, err)
	}
}

func TestRunner_Run(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		arrange func(_ *testing.T) (*cmd.Runner, cli.Options)
		assert  assertFn
	}{
		{
			name: "nil runner",
			arrange: func(_ *testing.T) (*cmd.Runner, cli.Options) {
				return nil, cli.Options{Provider: "dummy", MaxJobs: 1}
			},
			assert: assertErrContains("nil runner"),
		},
		{
			name: "missing service",
			arrange: func(_ *testing.T) (*cmd.Runner, cli.Options) {
				return cmd.NewRunner(nil), cli.Options{Provider: "dummy", MaxJobs: 1}
			},
			assert: assertErrContains("missing service"),
		},
		{
			name: "bad opts",
			arrange: func(_ *testing.T) (*cmd.Runner, cli.Options) {
				// real runner with service, but missing provider
				s := &mockService{}
				return cmd.NewRunner(s), cli.Options{Provider: "", MaxJobs: 1}
			},
			assert: assertErrContains("provider"),
		},
		{
			name: "happy path apply=false",
			arrange: func(_ *testing.T) (*cmd.Runner, cli.Options) {
				// Use a local mock Service and assert delegation
				s := &mockService{}
				s.On("Run", mock.Anything, 1, false).Return(1, nil).Once()
				return cmd.NewRunner(s), cli.Options{Provider: "dummy", MaxJobs: 1, Apply: false}
			},
			assert: assertNoError(),
		},
		{
			name: "happy path apply=true (upserts)",
			arrange: func(_ *testing.T) (*cmd.Runner, cli.Options) {
				s := &mockService{}
				s.On("Run", mock.Anything, 1, true).Return(2, nil).Once()
				return cmd.NewRunner(s), cli.Options{Provider: "dummy", MaxJobs: 1, Apply: true}
			},
			assert: assertNoError(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, opts := tc.arrange(t)
			var err error
			if r == nil {
				var rn *cmd.Runner
				err = rn.Run(context.Background(), opts)
			} else {
				err = r.Run(context.Background(), opts)
			}
			tc.assert(t, err)
		})
	}
}

// mockService is a lightweight testify-based mock for cmd.Service
type mockService struct{ mock.Mock }

func (m *mockService) Run(ctx context.Context, maxJobs int, apply bool) (int, error) {
	args := m.Called(ctx, maxJobs, apply)
	return args.Int(0), args.Error(1)
}
