package exportdataset_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	formatsPkg "github.com/umayangag/cric-flow/go-app/internal/formats"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

func mkCfg(req string) *config.Config {
	c := &config.Config{}
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
			cfg:  mkCfg(""),
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
			cfg:  mkCfg(" odi "),
			want: []string{"ODI"},
		},
		{
			name: "no required format falls back to every canonical format",
			cfg:  mkCfg(""),
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
