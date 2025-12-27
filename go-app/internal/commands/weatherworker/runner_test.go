package weatherworker_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherworker"
    cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/weatherworker"
    jobmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherworker/internal/mocks"
    svcmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherworker/internal/mocks"
    svcpkg "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherworker"
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
                return nil, cli.Options{Provider: "dummy", MaxJobs: 1}
            },
            assert: assertErrContains("nil runner"),
        },
        {
            name: "missing service",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                return cmd.NewRunner(nil), cli.Options{Provider: "dummy", MaxJobs: 1}
            },
            assert: assertErrContains("missing service"),
        },
        {
            name: "bad opts",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                // real runner with service, but missing provider
                s := &svcpkg.Service{}
                return cmd.NewRunner(s), cli.Options{Provider: "", MaxJobs: 1}
            },
            assert: assertErrContains("provider"),
        },
        {
            name: "happy path apply=false",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                // Build a concrete service with mocked dependencies
                jm := jobmocks.NewMockSource(t)
                pm := svcmocks.NewMockProvider(t)
                rm := svcmocks.NewMockRepository(t)
                // Expect one batch with one ID and then exhaustion
                jm.EXPECT().Next(mock.Anything, mock.AnythingOfType("int")).Return([]int64{7}, true, nil).Once()
                jm.EXPECT().Next(mock.Anything, mock.AnythingOfType("int")).Return(nil, false, nil).Once()
                // Expect a fetch for that ID
                pm.EXPECT().Fetch(mock.Anything, int64(7)).Return(nil, nil)
                s := svcpkg.NewService(jm, pm, rm)
                return cmd.NewRunner(s), cli.Options{Provider: "dummy", MaxJobs: 1, Apply: false}
            },
            assert: assertNoError(),
        },
        {
            name: "happy path apply=true (upserts)",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                jm := jobmocks.NewMockSource(t)
                pm := svcmocks.NewMockProvider(t)
                rm := svcmocks.NewMockRepository(t)
                jm.EXPECT().Next(mock.Anything, mock.AnythingOfType("int")).Return([]int64{11}, true, nil).Once()
                jm.EXPECT().Next(mock.Anything, mock.AnythingOfType("int")).Return(nil, false, nil).Once()
                // Two records upserted
                pm.EXPECT().Fetch(mock.Anything, int64(11)).Return([]svcpkg.WeatherData{{MatchID: 11}, {MatchID: 11}}, nil)
                rm.EXPECT().UpsertWeather(mock.Anything, mock.Anything).Return(nil).Twice()
                s := svcpkg.NewService(jm, pm, rm)
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
