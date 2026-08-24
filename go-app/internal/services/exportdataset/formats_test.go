package exportdataset_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	formatsPkg "github.com/umayangag/cric-flow/go-app/internal/formats"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

func mkCfg(split bool, req string) *config.Config {
	c := &config.Config{}
	c.Export.SplitByFormat = split
	c.Export.RequiredFormat = req
	return c
}

func TestResolveFormats_CliPrecedence(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		opts svc.Options
		cfg  *config.Config
		want []string
	}{
		{
			name: "all formats",
			opts: svc.Options{Formats: []string{"TEST", "ODI", "T20", "T20I"}},
			cfg:  mkCfg(false, ""),
			want: []string{"TEST", "ODI", "T20", "T20I"},
		},
		{
			name: "csv formats normalized",
			opts: svc.Options{Formats: []string{"ODI", "TEST"}},
			cfg:  nil,
			want: []string{"ODI", "TEST"},
		},
		{
			name: "single format",
			opts: svc.Options{Formats: []string{"T20I"}},
			cfg:  nil,
			want: []string{"T20I"},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := svc.ResolveFormats(tc.opts, tc.cfg)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestResolveFormats_ConfigFallbacks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		cfg  *config.Config
		want []string
	}{
		{
			name: "required format",
			cfg:  mkCfg(false, " odi "),
			want: []string{"ODI"},
		},
		{
			// split_by_format no longer changes the outcome: exports are always
			// per-format since the combined unsuffixed CSVs were removed in C3-1
			name: "split by format",
			cfg:  mkCfg(true, ""),
			want: formatsPkg.CanonicalCodes(),
		},
		{
			name: "no required format falls back to every canonical format",
			cfg:  mkCfg(false, ""),
			want: formatsPkg.CanonicalCodes(),
		},
		{
			name: "nil cfg falls back to every canonical format",
			cfg:  nil,
			want: formatsPkg.CanonicalCodes(),
		},
	}

	emptyOpts := svc.Options{}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := svc.ResolveFormats(emptyOpts, tc.cfg)
			require.Equal(t, tc.want, got)
		})
	}
}
