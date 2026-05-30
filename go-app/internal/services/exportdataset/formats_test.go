package exportdataset_test

import (
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

func mkCfg(split bool, req string) *config.Config {
	c := &config.Config{}
	c.Export.SplitByFormat = split
	c.Export.RequiredFormat = req
	return c
}

type assertStrsFn func(t *testing.T, got []string)

func assertEqualSlice(want []string) assertStrsFn {
	return func(t *testing.T, got []string) {
		require.Equal(t, len(want), len(got))
		for i := range want {
			require.Equal(t, got[i], want[i])
		}
	}
}

func TestResolveFormats_CliPrecedence(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		opts   svc.Options
		cfg    *config.Config
		want   []string
		assert assertStrsFn
	}{
		{
			name:   "all-formats",
			opts:   svc.Options{Formats: []string{"TEST", "ODI", "T20", "T20I"}},
			cfg:    mkCfg(false, ""),
			want:   []string{"TEST", "ODI", "T20", "T20I"},
			assert: assertEqualSlice([]string{"TEST", "ODI", "T20", "T20I"}),
		},
		{
			name:   "csv formats normalized",
			opts:   svc.Options{Formats: []string{"ODI", "TEST"}},
			cfg:    nil,
			want:   []string{"ODI", "TEST"},
			assert: assertEqualSlice([]string{"ODI", "TEST"}),
		},
		{
			name:   "single format",
			opts:   svc.Options{Formats: []string{"T20I"}},
			cfg:    nil,
			want:   []string{"T20I"},
			assert: assertEqualSlice([]string{"T20I"}),
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := svc.ResolveFormats(tc.opts, tc.cfg)
			tc.assert(t, got)
		})
	}
}

func TestResolveFormats_ConfigFallbacks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		cfg    *config.Config
		want   []string
		assert assertStrsFn
	}{
		{
			name:   "required format",
			cfg:    mkCfg(false, " odi "),
			want:   []string{"ODI"},
			assert: assertEqualSlice([]string{"ODI"}),
		},
		{
			name:   "split by format",
			cfg:    mkCfg(true, ""),
			want:   []string{"TEST", "ODI", "T20", "T20I"},
			assert: assertEqualSlice([]string{"TEST", "ODI", "T20", "T20I"}),
		},
		{
			name:   "legacy combined",
			cfg:    mkCfg(false, ""),
			want:   []string{""},
			assert: assertEqualSlice([]string{""}),
		},
		{
			name:   "nil cfg legacy combined",
			cfg:    nil,
			want:   []string{""},
			assert: assertEqualSlice([]string{""}),
		},
	}

	emptyOpts := svc.Options{}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := svc.ResolveFormats(emptyOpts, tc.cfg)
			tc.assert(t, got)
		})
	}
}
