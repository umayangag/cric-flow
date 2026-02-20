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
	cached.Outputs.ExportDir = "/tmp/export"

	require.Equal(t, "/tmp/cricsheet", DefaultCricsheetDir())
	require.Equal(t, "/tmp/export", DefaultExportDir())
}

// Not parallel: uses and mutates the package-level cached config.
func TestDefaultDirs_FallbacksWhenUnset(t *testing.T) {
	cached = &Config{} // simulate empty config loaded
	wantCricsheet := filepath.Join("..", "data", "go-app", "cricsheet")
	wantExport := filepath.Join("output", "go-app")

	require.Equal(t, wantCricsheet, DefaultCricsheetDir())
	require.Equal(t, wantExport, DefaultExportDir())
}

// Not parallel: sets env and mutates cached config.
func TestDefaultExportDir_EnvOverridesConfig(t *testing.T) {
	cached = &Config{}
	cached.Outputs.ExportDir = "/tmp/export"
	t.Setenv("GO_APP_OUTPUT_DIR", "/output/go-app")
	defer t.Setenv("GO_APP_OUTPUT_DIR", "")

	require.Equal(t, "/output/go-app", DefaultExportDir())
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

func TestExportTimeout(t *testing.T) {
	cached = nil
	cached = &Config{}
	defer func() { cached = nil }()

	// no config: ExportTimeout falls back to PipelineTimeout (0)
	d := ExportTimeout()
	require.Equal(t, time.Duration(0), d)

	// export_timeout_ms set: use it
	cached.Features.ExportTimeoutMs = 120000
	d = ExportTimeout()
	require.Equal(t, 120*time.Second, d)

	// export_timeout_ms 0: fall back to pipeline timeout
	cached.Features.ExportTimeoutMs = 0
	cached.Features.PrecomputeTimeoutMs = 90000
	d = ExportTimeout()
	require.Equal(t, 90*time.Second, d)
}

func TestEffectiveExportMaxMatchIDs(t *testing.T) {
	require.Equal(t, DefaultExportMaxMatchIDs, EffectiveExportMaxMatchIDs(nil))
	cfg := &Config{}
	cfg.Backtest.ExportMaxMatchIDs = 500
	require.Equal(t, 500, EffectiveExportMaxMatchIDs(cfg))
	cfg.Backtest.ExportMaxMatchIDs = 0
	require.Equal(t, DefaultExportMaxMatchIDs, EffectiveExportMaxMatchIDs(cfg))
}

func TestEffectiveMaxTotalSamples(t *testing.T) {
	require.Equal(t, DefaultMaxTotalSamples, EffectiveMaxTotalSamples(nil))
	cfg := &Config{}
	cfg.Predictor.MaxTotalSamples = 50000
	require.Equal(t, 50000, EffectiveMaxTotalSamples(cfg))
	cfg.Predictor.MaxTotalSamples = 0
	require.Equal(t, DefaultMaxTotalSamples, EffectiveMaxTotalSamples(cfg))
}

func TestEffectiveSimulationTopKPerTeam(t *testing.T) {
	require.Equal(t, DefaultSimulationTopKPerTeam, EffectiveSimulationTopKPerTeam(nil))
	cfg := &Config{}
	cfg.Predictor.SimulationTopKPerTeam = 30
	require.Equal(t, 30, EffectiveSimulationTopKPerTeam(cfg))
	cfg.Predictor.SimulationTopKPerTeam = 0
	require.Equal(t, DefaultSimulationTopKPerTeam, EffectiveSimulationTopKPerTeam(cfg))
}

func TestEffectiveSimulationNumSamplesPerMatchup(t *testing.T) {
	require.Equal(t, DefaultSimulationNumSamplesPerMatchup, EffectiveSimulationNumSamplesPerMatchup(nil))
	cfg := &Config{}
	cfg.Predictor.SimulationNumSamplesPerMatchup = 200
	require.Equal(t, 200, EffectiveSimulationNumSamplesPerMatchup(cfg))
	cfg.Predictor.SimulationNumSamplesPerMatchup = 0
	require.Equal(t, DefaultSimulationNumSamplesPerMatchup, EffectiveSimulationNumSamplesPerMatchup(cfg))
}

func TestEffectiveSimulationCVs(t *testing.T) {
	r, w, e := EffectiveSimulationCVs(nil)
	require.Equal(t, DefaultSimulationRunsCV, r)
	require.Equal(t, DefaultSimulationWicketsCV, w)
	require.Equal(t, DefaultSimulationEconomyCV, e)

	cfg := &Config{}
	cfg.Predictor.Simulation = &SimulationParams{RunsCV: 0.4, WicketsCV: 0.5, EconomyCV: 0.2}
	r, w, e = EffectiveSimulationCVs(cfg)
	require.Equal(t, 0.4, r)
	require.Equal(t, 0.5, w)
	require.Equal(t, 0.2, e)

	cfg.Predictor.Simulation = &SimulationParams{RunsCV: 0.3} // partial: others stay default
	r, w, e = EffectiveSimulationCVs(cfg)
	require.Equal(t, 0.3, r)
	require.Equal(t, DefaultSimulationWicketsCV, w)
	require.Equal(t, DefaultSimulationEconomyCV, e)
}
