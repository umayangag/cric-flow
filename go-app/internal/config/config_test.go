package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestEffectiveScoreNormParams(t *testing.T) {
	// nil config -> defaults (returns float64)
	bat, wkt, econ, fld := EffectiveScoreNormParams(nil, "T20")
	require.Equal(t, float64(DefaultScoreNormBatDivisor), bat)
	require.Equal(t, float64(DefaultScoreNormWicketDivisor), wkt)
	require.Equal(t, float64(DefaultScoreNormEconBase), econ)
	require.Equal(t, float64(DefaultScoreNormFieldDivisor), fld)

	// format not in config -> defaults
	cfg := &Config{}
	bat, wkt, econ, fld = EffectiveScoreNormParams(cfg, "T20")
	require.Equal(t, float64(DefaultScoreNormBatDivisor), bat)
	require.Equal(t, float64(DefaultScoreNormWicketDivisor), wkt)
	require.Equal(t, float64(DefaultScoreNormEconBase), econ)
	require.Equal(t, float64(DefaultScoreNormFieldDivisor), fld)

	// format in config -> overrides
	cfg.Selection.ScoreNormalization = map[string]ScoreNormParams{
		"T20": {BatDivisor: 100, WicketDivisor: 6, EconBase: 10, FieldDivisor: 4},
	}
	bat, wkt, econ, fld = EffectiveScoreNormParams(cfg, "T20")
	require.Equal(t, 100.0, bat)
	require.Equal(t, 6.0, wkt)
	require.Equal(t, 10.0, econ)
	require.Equal(t, 4.0, fld)

	// partial override: only bat/wicket set, econ/field stay default
	cfg.Selection.ScoreNormalization = map[string]ScoreNormParams{
		"ODI": {BatDivisor: 50},
	}
	bat, wkt, econ, fld = EffectiveScoreNormParams(cfg, "ODI")
	require.Equal(t, 50.0, bat)
	require.Equal(t, float64(DefaultScoreNormWicketDivisor), wkt)
	require.Equal(t, float64(DefaultScoreNormEconBase), econ)
	require.Equal(t, float64(DefaultScoreNormFieldDivisor), fld)
}

func TestEffectiveScoreWeights(t *testing.T) {
	// nil config -> defaults
	bat, bowl, field, keeper := EffectiveScoreWeights(nil)
	require.Equal(t, DefaultScoreWeightBat, bat)
	require.Equal(t, DefaultScoreWeightBowl, bowl)
	require.Equal(t, DefaultScoreWeightField, field)
	require.Equal(t, DefaultScoreWeightKeeperBonus, keeper)

	// config with custom weights
	cfg := &Config{}
	cfg.Selection.ScoreWeights = &ScoreWeights{Bat: 0.5, Bowl: 0.35, Field: 0.12, KeeperBonus: 0.03}
	bat, bowl, field, keeper = EffectiveScoreWeights(cfg)
	require.Equal(t, 0.5, bat)
	require.Equal(t, 0.35, bowl)
	require.Equal(t, 0.12, field)
	require.Equal(t, 0.03, keeper)

	// partial config: zero values filled with defaults
	cfg.Selection.ScoreWeights = &ScoreWeights{Bat: 0.6}
	bat, bowl, field, keeper = EffectiveScoreWeights(cfg)
	require.Equal(t, 0.6, bat)
	require.Equal(t, DefaultScoreWeightBowl, bowl)
	require.Equal(t, DefaultScoreWeightField, field)
	require.Equal(t, DefaultScoreWeightKeeperBonus, keeper)
}

func TestEffectiveScoreWeightsForFormat(t *testing.T) {
	// nil config -> falls through to EffectiveScoreWeights defaults
	bat, bowl, field, keeper := EffectiveScoreWeightsForFormat(nil, "T20")
	require.Equal(t, DefaultScoreWeightBat, bat)
	require.Equal(t, DefaultScoreWeightBowl, bowl)
	require.Equal(t, DefaultScoreWeightField, field)
	require.Equal(t, DefaultScoreWeightKeeperBonus, keeper)

	// score_weights_by_format override
	cfg := &Config{}
	cfg.Selection.ScoreWeightsByFormat = map[string]ScoreWeights{
		"T20": {Bat: 0.5, Bowl: 0.38, Field: 0.09, KeeperBonus: 0.03},
	}
	bat, bowl, field, keeper = EffectiveScoreWeightsForFormat(cfg, "T20")
	require.Equal(t, 0.5, bat)
	require.Equal(t, 0.38, bowl)
	require.Equal(t, 0.09, field)
	require.Equal(t, 0.03, keeper)
}

func TestPipelineTimeout(t *testing.T) {
	cached = nil
	cached = &Config{}
	defer func() { cached = nil }()

	// no timeout configured
	d := PipelineTimeout()
	require.Equal(t, time.Duration(0), d)

	// configured timeout
	cached.Features.PrecomputeTimeoutMs = 60000
	d = PipelineTimeout()
	require.Equal(t, 60*time.Second, d)
}
