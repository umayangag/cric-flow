package cricsheetimporter_test

import (
	"flag"
	"os"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/cricsheetimporter"
)

type assertFn func(t *testing.T, got cli.Options, err error)

func assertNoErrorApplyDir(wantDir string, wantApply bool, wantConc int) assertFn {
	return func(t *testing.T, got cli.Options, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.InDir != wantDir {
			t.Fatalf("want InDir=%q got %q", wantDir, got.InDir)
		}
		if got.Apply != wantApply {
			t.Fatalf("want Apply=%v got %v", wantApply, got.Apply)
		}
		if got.Concurrency != wantConc {
			t.Fatalf("want Concurrency=%d got %d", wantConc, got.Concurrency)
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
	tmp := t.TempDir()
	cases := []struct {
		name   string
		setup  func()
		args   []string
		assert assertFn
	}{
		{
			name:   "explicit flags override env/defaults",
			setup:  func() { os.Setenv("GO_APP_CRICSHEET_DIR", ""); os.Setenv("CRICSHEET_CONCURRENCY", "") },
			args:   []string{"-in", tmp, "-apply", "-concurrency", "8"},
			assert: assertNoErrorApplyDir(tmp, true, 8),
		},
		{
			name:   "env provides defaults when flags absent",
			setup:  func() { os.Setenv("GO_APP_CRICSHEET_DIR", tmp); os.Setenv("CRICSHEET_CONCURRENCY", "5") },
			args:   []string{},
			assert: assertNoErrorApplyDir(tmp, false, 5),
		},
		{
			name:   "error on empty in dir",
			setup:  func() { os.Setenv("GO_APP_CRICSHEET_DIR", " "); os.Setenv("CRICSHEET_CONCURRENCY", "") },
			args:   []string{"-in", ""},
			assert: assertErrorContains("input directory"),
		},
		{
			name:   "error on bad concurrency",
			setup:  func() { os.Setenv("GO_APP_CRICSHEET_DIR", tmp); os.Setenv("CRICSHEET_CONCURRENCY", "") },
			args:   []string{"-concurrency", "0"},
			assert: assertErrorContains("concurrency"),
		},
		{
			name:  "legacy behavior flags parsed",
			setup: func() { os.Setenv("GO_APP_CRICSHEET_DIR", tmp); os.Setenv("CRICSHEET_CONCURRENCY", "2") },
			args:  []string{"-placeholders-weather", "-placeholders-fielding", "-weather-enqueue=false"},
			assert: func(t *testing.T, got cli.Options, err error) {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if !got.PlaceholdersWeather {
					t.Fatalf("expected placeholders-weather true")
				}
				if !got.PlaceholdersFielding {
					t.Fatalf("expected placeholders-fielding true")
				}
				if got.WeatherEnqueue {
					t.Fatalf("expected weather-enqueue false")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// reset environment per case
			os.Unsetenv("GO_APP_CRICSHEET_DIR")
			os.Unsetenv("CRICSHEET_CONCURRENCY")
			if tc.setup != nil {
				tc.setup()
			}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := cli.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
