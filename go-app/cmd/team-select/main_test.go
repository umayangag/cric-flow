package main_test

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// TestParseArgs_DefaultsAndOverrides follows the gold-standard: external package,
// table-driven, Arrange → Act → Assert, and `require` assertions.
func TestParseArgs_DefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	type arrangeFn func() (fs *flag.FlagSet, args []string)
	type assertFn func(t *testing.T, got cli.Options, err error)

	testCases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "defaults applied (format T20, size=11, minBowlers=5)",
			arrange: func() (*flag.FlagSet, []string) {
				return flag.NewFlagSet("test", flag.ContinueOnError), []string{"-match=262039498036", "-season=2025"}
			},
			assert: func(t *testing.T, got cli.Options, err error) {
				require.NoError(t, err)
				require.Equal(t, cli.Options{
					MatchID:    262039498036,
					Format:     "T20",
					Season:     "2025",
					TeamSize:   11,
					MinBowlers: 5,
				}, got)
			},
		},
		{
			name: "overrides respected (ODI, keeper)",
			arrange: func() (*flag.FlagSet, []string) {
				return flag.NewFlagSet("test", flag.ContinueOnError), []string{
					"-match=1", "-season=2019", "-format=ODI",
					"-size=9", "-min-bowlers=4", "-require-keeper",
				}
			},
			assert: func(t *testing.T, got cli.Options, err error) {
				require.NoError(t, err)
				require.Equal(t, cli.Options{
					MatchID:       1,
					Format:        "ODI",
					Season:        "2019",
					TeamSize:      9,
					MinBowlers:    4,
					RequireKeeper: true,
				}, got)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			fs, args := tc.arrange()
			// Act
			got, err := cli.ParseArgs(fs, args)
			// Assert
			tc.assert(t, got, err)
		})
	}
}

func TestParseArgs_Errors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{name: "missing all", args: []string{}},
		{name: "missing match", args: []string{"-season=2025"}},
		{name: "missing season", args: []string{"-match=1"}},
		{name: "invalid format", args: []string{"-match=1", "-format=X", "-season=2019"}},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			_, err := cli.ParseArgs(fs, tc.args)
			require.Error(t, err)
		})
	}
}
