package backfillfielding_test

import (
	"flag"
	"os"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/backfillfielding"
)

type assertFn func(t *testing.T, got cli.Options, err error)

type assertErrFn func(t *testing.T, err error)

func assertNoErrorOpts(want cli.Options) assertFn {
	return func(t *testing.T, got cli.Options, err error) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		if got.All != want.All { t.Fatalf("want All=%v got %v", want.All, got.All) }
		if got.MatchID != want.MatchID { t.Fatalf("want MatchID=%d got %d", want.MatchID, got.MatchID) }
		if got.Apply != want.Apply { t.Fatalf("want Apply=%v got %v", want.Apply, got.Apply) }
		if got.Concurrency != want.Concurrency { t.Fatalf("want Concurrency=%d got %d", want.Concurrency, got.Concurrency) }
	}
}

func assertErrorContains(sub string) assertFn {
	return func(t *testing.T, _ cli.Options, err error) {
		s := ""
		if err != nil { s = err.Error() }
		if err == nil || indexOf(s, sub) < 0 { t.Fatalf("want err containing %q, got %v", sub, err) }
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ { if s[i+j] != sub[j] { ok = false; break } }
		if ok { return i }
	}
	return -1
}

func TestParseArgs_BasicAndEnvDefaults(t *testing.T) {
	t.Parallel()

	cases := []struct{
		name string
		setup func()
		args  []string
		assert assertFn
	}{
		{
			name: "all with defaults (env conc)",
			setup: func(){ os.Setenv("BACKFILL_CONCURRENCY", "6") },
			args:  []string{"--all"},
			assert: assertNoErrorOpts(cli.Options{All:true, MatchID:0, Apply:false, Concurrency:6}),
		},
		{
			name: "single match id and apply with explicit conc",
			setup: func(){ os.Setenv("BACKFILL_CONCURRENCY", "") },
			args:  []string{"--match","12345","--apply","--concurrency","3"},
			assert: assertNoErrorOpts(cli.Options{All:false, MatchID:12345, Apply:true, Concurrency:3}),
		},
		{
			name: "error when neither all nor match",
			setup: func(){ os.Unsetenv("BACKFILL_CONCURRENCY") },
			args:  []string{},
			assert: assertErrorContains("either --all or --match"),
		},
		{
			name: "error when both all and match",
			setup: func(){},
			args:  []string{"--all","--match","1"},
			assert: assertErrorContains("only one of --all or --match"),
		},
		{
			name: "error on bad match id",
			setup: func(){},
			args:  []string{"--match","abc"},
			assert: assertErrorContains("invalid match id"),
		},
		{
			name: "error on bad concurrency",
			setup: func(){},
			args:  []string{"--all","--concurrency","0"},
			assert: assertErrorContains("concurrency"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// reset env per test
			os.Unsetenv("BACKFILL_CONCURRENCY")
			if tc.setup != nil { tc.setup() }
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := cli.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
