package migrate_test

import (
	"flag"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/migrate"
)

type assertOptsFn func(t *testing.T, got svc.Options, err error)

func assertNoErrWant(want svc.Options) assertOptsFn {
	return func(t *testing.T, got svc.Options, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Dir != want.Dir {
			t.Fatalf("want Dir=%q got %q", want.Dir, got.Dir)
		}
	}
}

func assertOptsErr() assertOptsFn {
	return func(t *testing.T, _ svc.Options, err error) {
		t.Helper()
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
		assert assertOptsFn
	}{
		{
			name:   "defaults",
			fs:     nil,
			args:   []string{},
			assert: assertNoErrWant(svc.Options{Dir: "migrations"}),
		},
		{
			name:   "override dir",
			fs:     nil,
			args:   []string{"-dir", "/tmp/m"},
			assert: assertNoErrWant(svc.Options{Dir: "/tmp/m"}),
		},
		{
			name:   "unknown flag yields error",
			fs:     nil,
			args:   []string{"-x"},
			assert: assertOptsErr(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ParseArgs(tc.fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
