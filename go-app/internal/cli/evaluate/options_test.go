package evaluate_test

import (
	"flag"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/evaluate"
)

type assertOptsFn func(t *testing.T, got cli.Options, err error)

func assertNoErrWant(want cli.Options) assertOptsFn {
	return func(t *testing.T, got cli.Options, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Season != want.Season {
			t.Fatalf("want Season=%q got %q", want.Season, got.Season)
		}
		if got.Format != want.Format {
			t.Fatalf("want Format=%q got %q", want.Format, got.Format)
		}
	}
}

func assertErrContains(sub string) assertOptsFn {
	return func(t *testing.T, _ cli.Options, err error) {
		if err == nil {
			t.Fatalf("expected error containing %q; got nil", sub)
		}
		msg := err.Error()
		if !contains(msg, sub) {
			t.Fatalf("err %q does not contain %q", msg, sub)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func TestParseArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		args   []string
		assert assertOptsFn
	}{
		{
			name:   "defaults",
			args:   []string{},
			assert: assertNoErrWant(cli.Options{Season: "demo", Format: "T20"}),
		},
		{
			name:   "overrides",
			args:   []string{"-season", "2019", "-format", "ODI"},
			assert: assertNoErrWant(cli.Options{Season: "2019", Format: "ODI"}),
		},
		{
			name:   "missing season",
			args:   []string{"-season", "", "-format", "T20"},
			assert: assertErrContains("season must not be empty"),
		},
		{
			name:   "missing format",
			args:   []string{"-season", "2019", "-format", ""},
			assert: assertErrContains("format must not be empty"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := cli.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
