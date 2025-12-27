package backfillfielding_test

import (
    "context"
    "errors"
    "testing"

    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/backfillfielding"
    cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/backfillfielding"
    dbmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
    fsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/fielding"
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

func TestRunner_Run_Table(t *testing.T) {
    t.Parallel()

    cases := []struct {
        name    string
        arrange func(t *testing.T) (*cmd.Runner, cli.Options)
        assert  assertFn
    }{
        {
            name: "nil service errors",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                return &cmd.Runner{Svc: nil}, cli.Options{All: true, Apply: false, Concurrency: 1}
            },
            assert: assertErrContains("missing service"),
        },
        {
            name: "all path calls BackfillAll (nil match ptr)",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                m := dbmocks.NewMockFieldingRepo(t)
                // Expect ListFieldingEvents with nil match for --all path
                m.EXPECT().ListFieldingEvents(mock.Anything, (*int64)(nil)).Return([]fsvc.BackfillEvent{}, nil)
                s := fsvc.NewService(m)
                r := cmd.NewRunner(s)
                return r, cli.Options{All: true, Apply: false, Concurrency: 1}
            },
            assert: assertNoError(),
        },
        {
            name: "match path calls BackfillMatch (non-nil match ptr)",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                m := dbmocks.NewMockFieldingRepo(t)
                var matchID int64 = 42
                m.EXPECT().ListFieldingEvents(mock.Anything, &matchID).Return([]fsvc.BackfillEvent{}, nil)
                s := fsvc.NewService(m)
                r := cmd.NewRunner(s)
                return r, cli.Options{MatchID: 42, Apply: false}
            },
            assert: assertNoError(),
        },
        {
            name: "invalid match id",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                m := dbmocks.NewMockFieldingRepo(t)
                // No expectations; should fail fast before hitting repo
                s := fsvc.NewService(m)
                r := cmd.NewRunner(s)
                return r, cli.Options{MatchID: 0}
            },
            assert: assertErrContains("invalid match id"),
        },
        {
            name: "service error propagates",
            arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
                m := dbmocks.NewMockFieldingRepo(t)
                m.EXPECT().ListFieldingEvents(mock.Anything, (*int64)(nil)).Return(nil, errors.New("boom"))
                s := fsvc.NewService(m)
                r := cmd.NewRunner(s)
                return r, cli.Options{All: true, Apply: false, Concurrency: 1}
            },
            assert: assertErrContains("boom"),
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            r, opts := tc.arrange(t)
            err := r.Run(context.Background(), opts)
            tc.assert(t, err)
        })
    }
}
