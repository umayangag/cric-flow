package teamselect_test

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

type optsAssertFn func(t *testing.T, got svc.Options, err error)

func assertNoErrorOpts(want svc.Options) optsAssertFn {
	return func(t *testing.T, got svc.Options, err error) {
		require.NoError(t, err)
		require.Equal(t, want.MatchID, got.MatchID)
		require.Equal(t, want.Format, got.Format)
		require.Equal(t, want.Season, got.Season)
		require.Equal(t, want.TeamSize, got.TeamSize)
		require.Equal(t, want.MinBowlers, got.MinBowlers)
		require.Equal(t, want.RequireKeeper, got.RequireKeeper)
		require.Equal(t, want.FromDB, got.FromDB)
		require.Equal(t, want.PoolPath, got.PoolPath)
	}
}

func assertErrorContains(sub string) optsAssertFn {
	return func(t *testing.T, _ svc.Options, err error) {
		require.Error(t, err)
		require.Contains(t, err.Error(), sub)
	}
}

func TestParseArgs_Basic(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		setup  func()
		args   []string
		assert optsAssertFn
	}{
		{
			name:  "happy path from DB",
			setup: func() { os.Unsetenv("TEAM_SELECT_FROM_DB") },
			args: []string{
				"-match",
				"1193505",
				"-format",
				"T20",
				"-season",
				"2019",
				"-size",
				"11",
				"-min-bowlers",
				"5",
				"-require-keeper",
			},
			assert: assertNoErrorOpts(
				svc.Options{
					MatchID:       1193505,
					Format:        "T20",
					Season:        "2019",
					TeamSize:      11,
					MinBowlers:    5,
					RequireKeeper: true,
					FromDB:        true,
				},
			),
		},
		{
			name:  "happy path from CSV",
			setup: func() {},
			args: []string{
				"-match",
				"1193505",
				"-format",
				"ODI",
				"-season",
				"2019",
				"-size",
				"11",
				"-min-bowlers",
				"4",
				"-from-db=false",
				"-pool",
				"/tmp/pool.csv",
			},
			assert: assertNoErrorOpts(
				svc.Options{
					MatchID:    1193505,
					Format:     "ODI",
					Season:     "2019",
					TeamSize:   11,
					MinBowlers: 4,
					FromDB:     false,
					PoolPath:   "/tmp/pool.csv",
				},
			),
		},
		{
			name:   "invalid match",
			setup:  func() {},
			args:   []string{"-match", "0", "-format", "T20", "-season", "2019"},
			assert: assertErrorContains("match is required and must be a positive number"),
		},
		{
			name:   "invalid format",
			setup:  func() {},
			args:   []string{"-match", "1", "-format", "X", "-season", "2019"},
			assert: assertErrorContains("invalid format"),
		},
		{
			name:   "missing season",
			setup:  func() {},
			args:   []string{"-match", "1", "-format", "T20"},
			assert: assertErrorContains("season is required"),
		},
		{
			name:   "invalid size",
			setup:  func() {},
			args:   []string{"-match", "1", "-format", "T20", "-season", "2019", "-size", "0"},
			assert: assertErrorContains("team size must be a positive number"),
		},
		{
			name:   "invalid min-bowlers",
			setup:  func() {},
			args:   []string{"-match", "1", "-format", "T20", "-season", "2019", "-min-bowlers", "-1"},
			assert: assertErrorContains("min-bowlers must be a non-negative number"),
		},
		{
			name:   "from csv requires pool",
			setup:  func() {},
			args:   []string{"-match", "1", "-format", "T20", "-season", "2019", "-from-db=false"},
			assert: assertErrorContains("pool csv"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("TEAM_SELECT_FROM_DB")
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := svc.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
