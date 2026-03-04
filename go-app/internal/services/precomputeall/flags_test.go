package precomputeall_test

import (
	"flag"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/precomputeall"
)

func TestParseArgs_HappyPaths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
	}{
		{"replay_default", []string{"-format", "T20", "-replay"}},
		{"asof_with_seq", []string{"-format", "ODI", "-as-of", "2020-12-31", "-seq-targets", "all"}},
		{"aliases_ok", []string{"-format", "IT20", "-replay", "-ewm-alpha", "0.5", "-lastN", "12"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			if _, err := svc.ParseArgs(fs, tc.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseArgs_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
	}{
		{"empty_format", []string{"-format", "", "-replay"}},
		{"bad_alpha", []string{"-format", "T20", "-ewm-alpha", "0"}},
		{"neg_lastN", []string{"-format", "T20", "-lastN", "-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			if _, err := svc.ParseArgs(fs, tc.args); err == nil {
				t.Fatalf("expected error, got nil for %s", tc.name)
			}
		})
	}
}
