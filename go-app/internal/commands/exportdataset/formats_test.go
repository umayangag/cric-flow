package exportdataset_test

import (
	"testing"

	cli "github.com/umayangag/cric-flow/go-app/internal/cli/exportdataset"
	cmd "github.com/umayangag/cric-flow/go-app/internal/commands/exportdataset"
	"github.com/umayangag/cric-flow/go-app/internal/config"
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
		if len(got) != len(want) {
			t.Fatalf("want len=%d got len=%d (%v)", len(want), len(got), got)
		}
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("at %d: want %q got %q (full got=%v)", i, want[i], got[i], got)
			}
		}
	}
}

func TestResolveFormats_CliPrecedence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		opts   cli.Options
		cfg    *config.Config
		want   []string
		assert assertStrsFn
	}{
		{
			name:   "all-formats",
			opts:   cli.Options{Formats: []string{"TEST", "ODI", "T20", "T20I"}},
			cfg:    mkCfg(false, ""),
			want:   []string{"TEST", "ODI", "T20", "T20I"},
			assert: assertEqualSlice([]string{"TEST", "ODI", "T20", "T20I"}),
		},
		{
			name:   "csv formats normalized",
			opts:   cli.Options{Formats: []string{"ODI", "TEST"}},
			cfg:    nil,
			want:   []string{"ODI", "TEST"},
			assert: assertEqualSlice([]string{"ODI", "TEST"}),
		},
		{
			name:   "single format",
			opts:   cli.Options{Formats: []string{"T20I"}},
			cfg:    nil,
			want:   []string{"T20I"},
			assert: assertEqualSlice([]string{"T20I"}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cmd.ResolveFormats(tc.opts, tc.cfg)
			tc.assert(t, got)
		})
	}
}

func TestResolveFormats_ConfigFallbacks(t *testing.T) {
	t.Parallel()

	cases := []struct {
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

	emptyOpts := cli.Options{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cmd.ResolveFormats(emptyOpts, tc.cfg)
			tc.assert(t, got)
		})
	}
}
