package teampredictor_test

import (
	"flag"
	"os"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/teampredictor"

	"github.com/stretchr/testify/require"
)

type assertFn func(t *testing.T, got svc.Options, err error)

func assertNoErrorOpts(want svc.Options) assertFn {
	return func(t *testing.T, got svc.Options, err error) {
		require.NoError(t, err)
		require.Equal(t, want.MatchID, got.MatchID)
		require.Equal(t, want.Format, got.Format)
		require.Equal(t, want.Season, got.Season)
		require.Equal(t, want.Bat, got.Bat)
		require.Equal(t, want.Bowl, got.Bowl)
	}
}

func assertErrorContains(sub string) assertFn {
	return func(t *testing.T, _ svc.Options, err error) {
		t.Helper()
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
		assert assertFn
	}{
		{
			name: "happy path explicit flags",
			setup: func() {
				os.Unsetenv("TEAM_PREDICTOR_MATCH")
				os.Unsetenv("TEAM_PREDICTOR_FORMAT")
				os.Unsetenv("TEAM_PREDICTOR_SEASON")
			},
			args:   []string{"-match", "1193505", "-format", "T20", "-season", "2019", "-bat", "6", "-bowl", "5"},
			assert: assertNoErrorOpts(svc.Options{MatchID: 1193505, Format: "T20", Season: "2019", Bat: 6, Bowl: 5}),
		},
		{
			name: "env defaults used",
			setup: func() {
				os.Setenv("TEAM_PREDICTOR_MATCH", "1193505")
				os.Setenv("TEAM_PREDICTOR_FORMAT", "ODI")
				os.Setenv("TEAM_PREDICTOR_SEASON", "2011")
			},
			args:   []string{"-bat", "4", "-bowl", "6"},
			assert: assertNoErrorOpts(svc.Options{MatchID: 1193505, Format: "ODI", Season: "2011", Bat: 4, Bowl: 6}),
		},
		{
			name:   "invalid match",
			setup:  func() {},
			args:   []string{"-match", "0", "-format", "T20", "-season", "2019"},
			assert: assertErrorContains("invalid match"),
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
			assert: assertErrorContains("season"),
		},
		{
			name:   "invalid bat",
			setup:  func() {},
			args:   []string{"-match", "1", "-format", "T20", "-season", "2019", "-bat", "-1"},
			assert: assertErrorContains("invalid bat"),
		},
		{
			name:   "invalid bowl",
			setup:  func() {},
			args:   []string{"-match", "1", "-format", "T20", "-season", "2019", "-bowl", "-1"},
			assert: assertErrorContains("invalid bowl"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("TEAM_PREDICTOR_MATCH")
			os.Unsetenv("TEAM_PREDICTOR_FORMAT")
			os.Unsetenv("TEAM_PREDICTOR_SEASON")
			if tc.setup != nil {
				tc.setup()
			}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := svc.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
