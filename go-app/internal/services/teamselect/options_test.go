package teamselect_test

import (
	"flag"
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
		args   []string
		assert optsAssertFn
	}{
		{
			name: "happy path",
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
				},
			),
		},
		{
			name: "format alias is canonicalised",
			args: []string{
				"-match",
				"1193505",
				"-format",
				"IT20",
				"-season",
				"2019",
				"-size",
				"11",
				"-min-bowlers",
				"4",
			},
			assert: assertNoErrorOpts(
				svc.Options{
					MatchID:    1193505,
					Format:     "T20I",
					Season:     "2019",
					TeamSize:   11,
					MinBowlers: 4,
				},
			),
		},
		{
			name:   "invalid match",
			args:   []string{"-match", "0", "-format", "T20", "-season", "2019"},
			assert: assertErrorContains("match is required and must be a positive number"),
		},
		{
			name:   "invalid format",
			args:   []string{"-match", "1", "-format", "X", "-season", "2019"},
			assert: assertErrorContains("invalid format"),
		},
		{
			name:   "missing season",
			args:   []string{"-match", "1", "-format", "T20"},
			assert: assertErrorContains("season is required"),
		},
		{
			name:   "invalid size",
			args:   []string{"-match", "1", "-format", "T20", "-season", "2019", "-size", "0"},
			assert: assertErrorContains("team size must be a positive number"),
		},
		{
			name:   "invalid min-bowlers",
			args:   []string{"-match", "1", "-format", "T20", "-season", "2019", "-min-bowlers", "-1"},
			assert: assertErrorContains("min-bowlers must be a non-negative number"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := svc.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
