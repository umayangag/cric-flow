package weatherworker_test

import (
	"flag"
	"os"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherworker"
)

type assertFn func(t *testing.T, got cli.Options, err error)

func assertNoErrorOpts(want cli.Options) assertFn {
	return func(t *testing.T, got cli.Options, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Provider != want.Provider {
			t.Fatalf("want Provider=%q got %q", want.Provider, got.Provider)
		}
		if got.Apply != want.Apply {
			t.Fatalf("want Apply=%v got %v", want.Apply, got.Apply)
		}
		if got.MaxJobs != want.MaxJobs {
			t.Fatalf("want MaxJobs=%d got %d", want.MaxJobs, got.MaxJobs)
		}
	}
}

func assertErrorContains(sub string) assertFn {
	return func(t *testing.T, _ cli.Options, err error) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q, got %v", sub, err)
		}
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func TestParseArgs_Basic(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		setup  func()
		args   []string
		assert assertFn
	}{
		{
			name:   "defaults from env",
			setup:  func() { os.Setenv("WEATHER_PROVIDER", "dummy") },
			args:   []string{"-apply", "-max", "10"},
			assert: assertNoErrorOpts(cli.Options{Provider: "dummy", Apply: true, MaxJobs: 10}),
		},
		{
			name:   "explicit provider overrides env",
			setup:  func() { os.Setenv("WEATHER_PROVIDER", "other") },
			args:   []string{"-provider", "dummy", "-max", "0"},
			assert: assertNoErrorOpts(cli.Options{Provider: "dummy", Apply: false, MaxJobs: 0}),
		},
		{
			name:   "error on empty provider",
			setup:  func() { os.Setenv("WEATHER_PROVIDER", " ") },
			args:   []string{},
			assert: assertErrorContains("provider"),
		},
		{
			name:   "error on invalid max",
			setup:  func() { os.Setenv("WEATHER_PROVIDER", "dummy") },
			args:   []string{"-max", "-1"},
			assert: assertErrorContains("invalid max"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("WEATHER_PROVIDER")
			if tc.setup != nil {
				tc.setup()
			}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := cli.ParseArgs(fs, tc.args)
			// run assert
			// (no ifs in test bodies)
			tc.assert(t, got, err)
		})
	}
}
