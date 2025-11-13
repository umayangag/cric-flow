package migrate_test

import (
	"flag"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/migrate"
)

type assertFn func(t *testing.T, got cli.Options, err error)

func assertNoErrWant(want cli.Options) assertFn {
	return func(t *testing.T, got cli.Options, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Dir != want.Dir {
			t.Fatalf("want Dir=%q got %q", want.Dir, got.Dir)
		}
	}
}

func assertErr() assertFn {
	return func(t *testing.T, _ cli.Options, err error) {
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	}
}

func TestParseArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		fs     *flag.FlagSet
		args   []string
		assert assertFn
	}{
		{
			name:   "defaults",
			fs:     nil,
			args:   []string{},
			assert: assertNoErrWant(cli.Options{Dir: "migrations"}),
		},
		{
			name:   "override dir",
			fs:     nil,
			args:   []string{"-dir", "/tmp/m"},
			assert: assertNoErrWant(cli.Options{Dir: "/tmp/m"}),
		},
		{
			name:   "unknown flag yields error",
			fs:     nil,
			args:   []string{"-x"},
			assert: assertErr(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cli.ParseArgs(tc.fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
