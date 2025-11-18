package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Not parallel: tests touch package-level cache and process env via other tests.
func TestValidateTeamSettings(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
		err  string
	}{
		{
			name: "valid settings",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 5
				c.Team.DefaultBatters = 6
				c.Team.DefaultBowlers = 5
				c.Predictor.TeamSize = 11
				c.Predictor.DefaultExtras = 4
				return c
			}(),
			err: "",
		},
		{
			name: "min bowlers < 1",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 0
				c.Predictor.TeamSize = 11
				return c
			}(),
			err: "min bowlers",
		},
		{
			name: "team size < min bowlers",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 6
				c.Predictor.TeamSize = 5
				return c
			}(),
			err: "team size",
		},
		{
			name: "negative extras",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 5
				c.Predictor.TeamSize = 11
				c.Predictor.DefaultExtras = -1
				return c
			}(),
			err: "extras",
		},
		{
			name: "default bowlers < min bowlers",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 5
				c.Team.DefaultBowlers = 3
				c.Predictor.TeamSize = 11
				return c
			}(),
			err: "default bowlers",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Act
			err := ValidateTeamSettings(tc.cfg)
			// Assert
			if tc.err == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tc.err))
		})
	}
}

// Not parallel: uses and mutates the package-level cached config.
func TestDefaultDirs_UseConfigValues(t *testing.T) {
	// ensure cached does not leak across tests
	cached = &Config{}
	cached.Inputs.CricsheetDir = "/tmp/cricsheet"
	cached.Inputs.EtlDir = "/tmp/etl"
	cached.Outputs.ExportDir = "/tmp/export"

	require.Equal(t, "/tmp/cricsheet", DefaultCricsheetDir())
	require.Equal(t, "/tmp/etl", DefaultEtlDir())
	require.Equal(t, "/tmp/export", DefaultExportDir())
}

// Not parallel: uses and mutates the package-level cached config.
func TestDefaultDirs_FallbacksWhenUnset(t *testing.T) {
	cached = &Config{} // simulate empty config loaded
	wantCricsheet := filepath.Join("..", "data", "go-app", "cricsheet")
	wantEtl := filepath.Join("..", "data", "go-app", "createdb")
	wantExport := filepath.Join("..", "output", "go-app")

	require.Equal(t, wantCricsheet, DefaultCricsheetDir())
	require.Equal(t, wantEtl, DefaultEtlDir())
	require.Equal(t, wantExport, DefaultExportDir())
}
