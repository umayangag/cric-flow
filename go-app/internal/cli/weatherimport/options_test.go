package weatherimport_test

import (
	"flag"
	"os"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherimport"
)

type assertFn func(t *testing.T, got cli.Options, err error)

type assertErrFn func(t *testing.T, err error)

func assertNoErrorOpts(match int64, provider string, apply bool) assertFn {
	return func(t *testing.T, got cli.Options, err error) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		if got.MatchID != match { t.Fatalf("want MatchID=%d got %d", match, got.MatchID) }
		if got.Provider != provider { t.Fatalf("want Provider=%q got %q", provider, got.Provider) }
		if got.Apply != apply { t.Fatalf("want Apply=%v got %v", apply, got.Apply) }
	}
}

func assertErrorContains(sub string) assertFn {
	return func(t *testing.T, _ cli.Options, err error) {
		s := ""
		if err != nil { s = err.Error() }
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q got %v", sub, err)
		}
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] { ok = false; break }
		}
		if ok { return i }
	}
	return -1
}

func TestParseArgs_Table(t *testing.T) {
	t.Parallel()

	cases := []struct{
		name string
		setup func()
		args []string
		assert assertFn
	}{
		{
			name: "explicit flags override env",
			setup: func(){ os.Setenv("WEATHER_PROVIDER", ""); },
			args: []string{"-match", "1193505", "-provider", "dummy", "-apply"},
			assert: assertNoErrorOpts(1193505, "dummy", true),
		},
		{
			name: "env default provider used when flag absent",
			setup: func(){ os.Setenv("WEATHER_PROVIDER", "mock"); },
			args: []string{"-match", "42"},
			assert: assertNoErrorOpts(42, "mock", false),
		},
		{
			name: "missing match id",
			setup: func(){ os.Setenv("WEATHER_PROVIDER", "dummy"); },
			args: []string{"-provider", "dummy"},
			assert: assertErrorContains("match id"),
		},
		{
			name: "invalid match id",
			setup: func(){ os.Setenv("WEATHER_PROVIDER", "dummy"); },
			args: []string{"-match", "abc"},
			assert: assertErrorContains("invalid match id"),
		},
		{
			name: "empty provider rejected",
			setup: func(){ os.Setenv("WEATHER_PROVIDER", " "); },
			args: []string{"-match", "1", "-provider", "  "},
			assert: assertErrorContains("provider"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("WEATHER_PROVIDER")
			if tc.setup != nil { tc.setup() }
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := cli.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
