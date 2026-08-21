package cricsheetimporter_test

import (
	"flag"
	"os"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/cricsheetimporter"

	"github.com/stretchr/testify/require"
)

type assertFn func(t *testing.T, got svc.Options, err error)

func assertNoErrorApplyDir(wantDir string, wantApply bool, wantConc int) assertFn {
	return func(t *testing.T, got svc.Options, err error) {
		t.Helper()
		require.NoError(t, err)
		require.Equal(t, wantDir, got.InDir)
		require.Equal(t, wantApply, got.Apply)
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
			assert: func(t *testing.T, got svc.Options, err error) {
				t.Helper()
				require.NoError(t, err)
				require.True(t, got.PlaceholdersWeather)
				require.True(t, got.PlaceholdersFielding)
				require.False(t, got.WeatherEnqueue)
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
