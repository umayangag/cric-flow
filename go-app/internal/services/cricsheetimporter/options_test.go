package cricsheetimporter_test

import (
	"flag"
	"os"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/cricsheetimporter"

	"github.com/stretchr/testify/require"
)

type assertFn func(t *testing.T, got svc.Options, err error)

func assertNoErrorDir(wantDir string, wantConc int) assertFn {
	return func(t *testing.T, got svc.Options, err error) {
		t.Helper()
		require.NoError(t, err)
		require.Equal(t, wantDir, got.InDir)
		require.Equal(t, wantConc, got.Concurrency)
	}
}

func assertErrorContains(sub string) assertFn {
	return func(t *testing.T, _ svc.Options, err error) {
		t.Helper()
		require.Error(t, err)
		require.Contains(t, err.Error(), sub)
	}
}

func TestParseArgs_Basic(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	testCases := []struct {
		name   string
		setup  func()
		args   []string
		assert assertFn
	}{
		{
			name:   "explicit flags override env/defaults",
			setup:  func() { os.Setenv("GO_APP_CRICSHEET_DIR", ""); os.Setenv("CRICSHEET_CONCURRENCY", "") },
			args:   []string{"-in", tmp, "-concurrency", "8"},
			assert: assertNoErrorDir(tmp, 8),
		},
		{
			name:   "env provides defaults when flags absent",
			setup:  func() { os.Setenv("GO_APP_CRICSHEET_DIR", tmp); os.Setenv("CRICSHEET_CONCURRENCY", "5") },
			args:   []string{},
			assert: assertNoErrorDir(tmp, 5),
		},
		{
			// IMPORT-10: "-apply" used to be parsed and silently ignored -- every run wrote
			// regardless of its value, so an operator expecting a dry run got a full,
			// irreversible import (IMPORT-03 made writes delete-then-insert). The flag is
			// deleted rather than honoured, so the CLI now refuses to start instead of lying
			// about what it is about to do.
			name:   "apply flag is refused, not silently accepted",
			setup:  func() { os.Setenv("GO_APP_CRICSHEET_DIR", tmp); os.Setenv("CRICSHEET_CONCURRENCY", "") },
			args:   []string{"-apply"},
			assert: assertErrorContains("flag provided but not defined: -apply"),
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
			args:  []string{"-placeholders-fielding"},
			assert: func(t *testing.T, got svc.Options, err error) {
				t.Helper()
				require.NoError(t, err)
				require.True(t, got.PlaceholdersFielding)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("GO_APP_CRICSHEET_DIR")
			os.Unsetenv("CRICSHEET_CONCURRENCY")
			if tc.setup != nil {
				tc.setup()
			}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := svc.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
