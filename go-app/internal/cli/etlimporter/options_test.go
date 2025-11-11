package etlimporter_test

import (
	"flag"
	"os"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/etlimporter"
)

type assertFn func(t *testing.T, got cli.Options, err error)

type assertErrFn func(t *testing.T, err error)

func assertNoErrorOpts(want cli.Options) assertFn {
	return func(t *testing.T, got cli.Options, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.InDir != want.InDir {
			t.Fatalf("want InDir=%q got %q", want.InDir, got.InDir)
		}
		if got.Apply != want.Apply {
			t.Fatalf("want Apply=%v got %v", want.Apply, got.Apply)
		}
		if got.Concurrency != want.Concurrency {
			t.Fatalf("want Concurrency=%d got %d", want.Concurrency, got.Concurrency)
		}
		if got.Pattern != want.Pattern {
			t.Fatalf("want Pattern=%q got %q", want.Pattern, got.Pattern)
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

func TestParseArgs_BasicAndEnvDefaults(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	cases := []struct {
		name   string
		setup  func()
		args   []string
		assert assertFn
	}{
		{
			name:  "explicit flags override env/defaults",
			setup: func() { os.Setenv("GO_APP_ETL_DIR", ""); os.Setenv("ETL_CONCURRENCY", ""); os.Setenv("ETL_PATTERN", "") },
			args:  []string{"-in", tmp, "-apply", "-concurrency", "8", "-pattern", "*.bat.csv"},
			assert: assertNoErrorOpts(cli.Options{InDir: tmp, Apply: true, Concurrency: 8, Pattern: "*.bat.csv"}),
		},
		{
			name:  "env provides defaults when flags absent",
			setup: func() { os.Setenv("GO_APP_ETL_DIR", tmp); os.Setenv("ETL_CONCURRENCY", "5"); os.Setenv("ETL_PATTERN", "*.csv") },
			args:  []string{},
			assert: assertNoErrorOpts(cli.Options{InDir: tmp, Apply: false, Concurrency: 5, Pattern: "*.csv"}),
		},
		{
			name:  "error on empty in dir",
			setup: func() { os.Setenv("GO_APP_ETL_DIR", " "); os.Setenv("ETL_CONCURRENCY", ""); os.Setenv("ETL_PATTERN", "*.csv") },
			args:  []string{"-in", ""},
			assert: assertErrorContains("input directory"),
		},
		{
			name:  "error on bad concurrency",
			setup: func() { os.Unsetenv("GO_APP_ETL_DIR"); os.Setenv("ETL_CONCURRENCY", ""); os.Setenv("ETL_PATTERN", "*.csv") },
			args:  []string{"-in", tmp, "-concurrency", "0"},
			assert: assertErrorContains("concurrency"),
		},
		{
			name:  "error on empty pattern",
			setup: func() { os.Unsetenv("GO_APP_ETL_DIR"); os.Setenv("ETL_CONCURRENCY", ""); os.Setenv("ETL_PATTERN", "") },
			args:  []string{"-in", tmp, "-pattern", "  "},
			assert: assertErrorContains("pattern"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// reset env per test
			os.Unsetenv("GO_APP_ETL_DIR")
			os.Unsetenv("ETL_CONCURRENCY")
			os.Unsetenv("ETL_PATTERN")
			if tc.setup != nil {
				tc.setup()
			}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := cli.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
