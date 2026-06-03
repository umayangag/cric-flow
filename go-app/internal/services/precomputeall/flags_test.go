package precomputeall_test

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/precomputeall"
)

func TestParseArgs_HappyPaths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{"replay default", []string{"-format", "T20", "-replay"}},
		{"asof with seq", []string{"-format", "ODI", "-as-of", "2020-12-31", "-seq-targets", "all"}},
		{"aliases ok", []string{"-format", "IT20", "-replay", "-ewm-alpha", "0.5", "-lastN", "12"}},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			_, err := svc.ParseArgs(fs, tc.args)
			require.NoError(t, err)
		})
	}
}

func TestParseArgs_Validation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{"empty format", []string{"-format", "", "-replay"}},
		{"bad alpha", []string{"-format", "T20", "-ewm-alpha", "0"}},
		{"neg lastN", []string{"-format", "T20", "-lastN", "-1"}},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			_, err := svc.ParseArgs(fs, tc.args)
			require.Error(t, err)
		})
	}
}
