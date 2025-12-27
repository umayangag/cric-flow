package etlimporter_test

import (
	"context"
	"testing"

	testmock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/etlimporter"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/etlimporter"
	mocks "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/etlimporter/internal/mocks"
	svcpkg "github.com/umayangag/cric-info-scrapers/go-app/internal/services/etlimporter"
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
		arrange func(t *testing.T) (*cmd.Runner, cli.Options)
		assert  assertFn
	}{
		{
			name: "nil runner",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				_ = t
				return nil, cli.Options{InDir: "/x", Concurrency: 1, Pattern: "*.csv"}
			},
			assert: assertErrContains("nil runner"),
		},
		{
			name: "missing service",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				_ = t
				return cmd.NewRunner(nil), cli.Options{InDir: "/x", Concurrency: 1, Pattern: "*.csv"}
			},
			assert: assertErrContains("missing service"),
		},
		{
			name: "bad opts",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				m := mocks.NewMockIngestor(t)
				r := cmd.NewRunner(m)
				return r, cli.Options{InDir: "", Concurrency: 0, Pattern: ""}
			},
			assert: assertErrContains("input directory"),
		},
		{
			name: "happy path calls service",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				m := mocks.NewMockIngestor(t)
				m.EXPECT().IngestDir(testmock.Anything, "/data", "*.csv", true, 1).Return(svcpkg.Stats{Files: 1}, nil)
				r := cmd.NewRunner(m)
				return r, cli.Options{InDir: "/data", Concurrency: 1, Pattern: "*.csv", Apply: true}
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
