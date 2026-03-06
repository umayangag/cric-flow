package config

import (
	"os"
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
		{
			name: "default batters negative",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 5
				c.Team.DefaultBatters = -1
				c.Predictor.TeamSize = 11
				return c
			}(),
			err: "default batters",
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

func TestConfigServerHelpers(t *testing.T) {
	// Server helpers return config value or default when nil/zero
	require.Equal(t, DefaultServerMLHealthTimeoutSec, ServerMLHealthTimeoutSec(nil))
	require.Equal(t, "http://localhost:8000", ServerMLBaseURLFallback(nil))
	require.Equal(t, DefaultServerListenAddress, ServerListenAddress(nil))

	cfg := &Config{}
	cfg.Server.MLHealthTimeoutSec = 15
	cfg.Server.MLBaseURLFallback = "http://ml:8000"
	cfg.Server.ListenAddress = ":9000"
	require.Equal(t, 15, ServerMLHealthTimeoutSec(cfg))
	require.Equal(t, "http://ml:8000", ServerMLBaseURLFallback(cfg))
	require.Equal(t, ":9000", ServerListenAddress(cfg))
}

func TestConfigBacktestAndOpsHelpers(t *testing.T) {
	require.Equal(t, DefaultBacktestListDefaultLimit, BacktestListDefaultLimit(nil))
	require.Equal(t, DefaultBacktestListMaxLimit, BacktestListMaxLimit(nil))
	require.Equal(t, DefaultOpsMigrationsPageDefault, OpsMigrationsPageDefault(nil))

	cfg := &Config{}
	cfg.Backtest.ListDefaultLimit = 25
	cfg.Backtest.ListMaxLimit = 100
	cfg.Ops.MigrationsPageDefault = 20
	require.Equal(t, 25, BacktestListDefaultLimit(cfg))
	require.Equal(t, 100, BacktestListMaxLimit(cfg))
	require.Equal(t, 20, OpsMigrationsPageDefault(cfg))
}

func TestPipelinePrecomputeETASecondsPerFmt(t *testing.T) {
	require.Equal(t, DefaultServerPrecomputeETASecPerFmt, PipelinePrecomputeETASecondsPerFmt(nil))

	cfg := &Config{}
	cfg.Pipeline.PrecomputeETASecondsPerFmt = 123
	require.Equal(t, 123, PipelinePrecomputeETASecondsPerFmt(cfg))
}

func TestConfigServerAndResourcesHelpers(t *testing.T) {
	cfg := &Config{}
	cfg.Server.ReadinessTimeoutSec = 5
	require.Equal(t, 5, ServerReadinessTimeoutSec(cfg))

	cfg.Resources = &ResourcesConfig{PrecomputeMBPerWorker: 600}
	require.Equal(t, 600, ResourcesPrecomputeMBPerWorker(cfg))

	cfg.Selection.MaxPoolSizeForFullEnum = 20
	require.Equal(t, 20, SelectionMaxPoolSizeForFullEnum(cfg))

	cfg.Selection.MaxWinProbSwapIterations = 30
	require.Equal(t, 30, SelectionMaxWinProbSwapIterations(cfg))

	cfg.Selection.MaxWinProbEvalBudget = 200
	require.Equal(t, 200, SelectionMaxWinProbEvalBudget(cfg))

	cfg.Pipeline.ReplayMatchPageSize = 250
	require.Equal(t, 250, PipelineReplayMatchPageSize(cfg))
}

func TestConfigBacktestJobHelpers(t *testing.T) {
	require.Equal(t, DefaultBacktestJobCleanupAgeHours, BacktestJobCleanupAgeHours(nil))
	require.Equal(t, DefaultBacktestJobCleanupIntervalMin, BacktestJobCleanupIntervalMin(nil))

	cfg := &Config{}
	cfg.Backtest.Job = &BacktestJobConfig{CleanupAgeHours: 48, CleanupIntervalMin: 30}
	require.Equal(t, 48, BacktestJobCleanupAgeHours(cfg))
	require.Equal(t, 30, BacktestJobCleanupIntervalMin(cfg))
}

func TestValidateTeamSettings_NilConfig(t *testing.T) {
	require.NoError(t, ValidateTeamSettings(nil))
}

func TestConfigServerGetters_AllDefaults(t *testing.T) {
	// Cover remaining Server getters that return defaults when nil or zero
	require.Equal(t, DefaultServerMLHealthBodyLimitBytes, ServerMLHealthBodyLimitBytes(nil))
	require.Equal(t, DefaultServerTrainStepTimeoutMin, ServerTrainStepTimeoutMin(nil))
	require.Equal(t, DefaultServerPipelineProgressSec, ServerPipelineProgressSec(nil))

	cfg := &Config{}
	cfg.Server.MLHealthBodyLimitBytes = 2097152
	cfg.Server.TrainStepTimeoutMin = 60
	cfg.Server.PipelineProgressSec = 5
	cfg.Server.DBProbeTimeoutSec = 3
	require.Equal(t, 2097152, ServerMLHealthBodyLimitBytes(cfg))
	require.Equal(t, 60, ServerTrainStepTimeoutMin(cfg))
	require.Equal(t, 5, ServerPipelineProgressSec(cfg))
	require.Equal(t, 3, ServerDBProbeTimeoutSec(cfg))
}

func TestEffectiveScoreWeightsForFormat_WithMetaModel(t *testing.T) {
	cached = nil
	tmp := t.TempDir()
	metaPath := filepath.Join(tmp, "meta.json")
	metaContent := `{"bat":0.4,"bowl":0.35,"field":0.2,"keeper_bonus":0.05}`
	if err := os.WriteFile(metaPath, []byte(metaContent), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	cfgPath := filepath.Join(tmp, "config.json")
	cfgContent := `{"selection":{"meta_model_path":"` + metaPath + `"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("GO_APP_CONFIG", cfgPath)
	cfg := Load()

	bat, bowl, field, keeper := EffectiveScoreWeightsForFormat(cfg, "T20")
	require.Equal(t, 0.4, bat)
	require.Equal(t, 0.35, bowl)
	require.Equal(t, 0.2, field)
	require.Equal(t, 0.05, keeper)
}

func TestEffectiveScoreWeightsForFormat_WithMetaModelPerFormat(t *testing.T) {
	cached = nil
	tmp := t.TempDir()
	metaPath := filepath.Join(tmp, "meta_per_fmt.json")
	metaContent := `{"bat":0.5,"bowl":0.3,"field":0.15,"keeper_bonus":0.05,"per_format":{"ODI":{"bat":0.45,"bowl":0.35,"field":0.15,"keeper_bonus":0.05}}}`
	if err := os.WriteFile(metaPath, []byte(metaContent), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	cfgPath := filepath.Join(tmp, "config.json")
	cfgContent := `{"selection":{"meta_model_path":"` + metaPath + `"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("GO_APP_CONFIG", cfgPath)
	cfg := Load()

	// ODI has per-format override
	bat, bowl, _, _ := EffectiveScoreWeightsForFormat(cfg, "ODI")
	require.Equal(t, 0.45, bat)
	require.Equal(t, 0.35, bowl)

	// T20 uses global meta-model (no per-format)
	bat2, bowl2, _, _ := EffectiveScoreWeightsForFormat(cfg, "T20")
	require.Equal(t, 0.5, bat2)
	require.Equal(t, 0.3, bowl2)
}

func TestConfigMoreServerAndBacktestHelpers(t *testing.T) {
	cfgEvalJob := &Config{}
	cfgEvalJob.Backtest.Job = &BacktestJobConfig{EvalJobMaxDurationHr: 8}
	cfgExportJob := &Config{}
	cfgExportJob.Backtest.Job = &BacktestJobConfig{ExportContributionsMaxDurHr: 4}
	cfgEvalConcurrency := &Config{}
	cfgEvalConcurrency.Backtest.Job = &BacktestJobConfig{
		EvalJobConcurrencyMin: 2,
		EvalJobConcurrencyMax: 4,
	}
	cfgServerTimeouts := &Config{}
	cfgServerTimeouts.Server.DBProbeLongTimeoutSec = 10
	cfgServerTimeouts.Server.ArtifactsTimeoutSec = 120
	cfgServerTimeouts.Server.HTTPReadTimeoutSec = 15
	cfgServerTimeouts.Server.HTTPWriteTimeoutSec = 20
	cfgServerTimeouts.Server.HTTPIdleTimeoutSec = 25
	cfgResources := &Config{
		Resources: &ResourcesConfig{
			ExportMBPerWorker:                256,
			FieldingMBPerWorker:              128,
			MemoryUsageFractionPercent:       75,
			SeqCalcLowMemoryLimitGiB:         8,
			PrecomputeConcurrencyWhenNoLimit: 6,
		},
	}

	tests := []struct {
		name string
		cfg  *Config
		fn   func(*Config) int
		want int
	}{
		{"ServerMLClientTimeoutSec nil", nil, ServerMLClientTimeoutSec, DefaultServerMLClientTimeoutSec},
		{
			"ServerMLClientTimeoutSec set",
			&Config{Server: ServerConfig{MLClientTimeoutSec: 25}},
			ServerMLClientTimeoutSec,
			25,
		},
		{"ServerDBProbeTimeoutSec nil", nil, ServerDBProbeTimeoutSec, DefaultServerDBProbeTimeoutSec},
		{
			"BacktestAccuracyTrendDefaultLimit nil",
			nil,
			BacktestAccuracyTrendDefaultLimit,
			DefaultBacktestAccuracyTrendLimit,
		},
		{
			"BacktestAccuracyTrendDefaultLimit set",
			func() *Config { c := &Config{}; c.Backtest.AccuracyTrendDefaultLimit = 30; return c }(),
			BacktestAccuracyTrendDefaultLimit,
			30,
		},
		{"BacktestEvalJobMaxDurationHr set", cfgEvalJob, BacktestEvalJobMaxDurationHr, 8},
		{"OpsMigrationsPageMax nil", nil, OpsMigrationsPageMax, DefaultOpsMigrationsPageMax},
		{
			"OpsMigrationsPageMax set",
			&Config{Ops: OpsConfig{MigrationsPageMax: 500}},
			OpsMigrationsPageMax,
			500,
		},
		{"OpsMigrationsPageCap set", &Config{Ops: OpsConfig{MigrationsPageCap: 5000}}, OpsMigrationsPageCap, 5000},
		{"BacktestAccuracyTrendMaxLimit nil", nil, BacktestAccuracyTrendMaxLimit, DefaultBacktestListMaxLimit},
		{
			"BacktestAccuracyTrendMaxLimit set",
			func() *Config { c := &Config{}; c.Backtest.AccuracyTrendMaxLimit = 200; return c }(),
			BacktestAccuracyTrendMaxLimit,
			200,
		},
		{"ResourcesSeqCalcMBPerWorker nil", nil, ResourcesSeqCalcMBPerWorker, DefaultSeqCalcMBPerWorker},
		{
			"ResourcesSeqCalcMBPerWorker set",
			&Config{Resources: &ResourcesConfig{SeqCalcMBPerWorker: 512}},
			ResourcesSeqCalcMBPerWorker,
			512,
		},
		{
			"OpsRecentMigrationsCount set",
			&Config{Ops: OpsConfig{RecentMigrationsCount: 50}},
			OpsRecentMigrationsCount,
			50,
		},
		{
			"ResourcesImportMBPerWorker set",
			&Config{Resources: &ResourcesConfig{ImportMBPerWorker: 200}},
			ResourcesImportMBPerWorker,
			200,
		},
		{
			"ResourcesExportMBPerWorker set",
			cfgResources,
			ResourcesExportMBPerWorker,
			256,
		},
		{
			"ResourcesFieldingMBPerWorker set",
			cfgResources,
			ResourcesFieldingMBPerWorker,
			128,
		},
		{
			"ResourcesMemoryUsageFractionPercent set",
			cfgResources,
			ResourcesMemoryUsageFractionPercent,
			75,
		},
		{
			"ResourcesSeqCalcLowMemoryLimitGiB set",
			cfgResources,
			ResourcesSeqCalcLowMemoryLimitGiB,
			8,
		},
		{
			"ResourcesPrecomputeConcurrencyWhenNoLimit set",
			cfgResources,
			ResourcesPrecomputeConcurrencyWhenNoLimit,
			6,
		},
		{
			"BacktestExportContributionsJobMaxDurationHr set",
			cfgExportJob,
			BacktestExportContributionsJobMaxDurationHr,
			4,
		},
		{
			"BacktestEvalJobConcurrencyMin set",
			cfgEvalConcurrency,
			BacktestEvalJobConcurrencyMin,
			2,
		},
		{
			"BacktestEvalJobConcurrencyMax set",
			cfgEvalConcurrency,
			BacktestEvalJobConcurrencyMax,
			4,
		},
		{
			"ServerDBProbeLongTimeoutSec default when nil",
			nil,
			ServerDBProbeLongTimeoutSec,
			DefaultServerDBProbeLongTimeoutSec,
		},
		{
			"ServerDBProbeLongTimeoutSec set",
			cfgServerTimeouts,
			ServerDBProbeLongTimeoutSec,
			10,
		},
		{
			"ServerArtifactsTimeoutSec default when nil",
			nil,
			ServerArtifactsTimeoutSec,
			DefaultServerArtifactsTimeoutSec,
		},
		{
			"ServerArtifactsTimeoutSec set",
			cfgServerTimeouts,
			ServerArtifactsTimeoutSec,
			120,
		},
		{
			"ServerHTTPReadTimeoutSec default when nil",
			nil,
			ServerHTTPReadTimeoutSec,
			DefaultServerHTTPReadTimeoutSec,
		},
		{
			"ServerHTTPReadTimeoutSec set",
			cfgServerTimeouts,
			ServerHTTPReadTimeoutSec,
			15,
		},
		{
			"ServerHTTPWriteTimeoutSec default when nil",
			nil,
			ServerHTTPWriteTimeoutSec,
			DefaultServerHTTPWriteTimeoutSec,
		},
		{
			"ServerHTTPWriteTimeoutSec set",
			cfgServerTimeouts,
			ServerHTTPWriteTimeoutSec,
			20,
		},
		{
			"ServerHTTPIdleTimeoutSec default when nil",
			nil,
			ServerHTTPIdleTimeoutSec,
			DefaultServerHTTPIdleTimeoutSec,
		},
		{
			"ServerHTTPIdleTimeoutSec set",
			cfgServerTimeouts,
			ServerHTTPIdleTimeoutSec,
			25,
		},
		{
			"BacktestExportContributionsJobMaxDurationHr nil",
			nil,
			BacktestExportContributionsJobMaxDurationHr,
			DefaultExportContributionsJobMaxDurHr,
		},
		{"BacktestEvalJobMaxDurationHr nil", nil, BacktestEvalJobMaxDurationHr, DefaultEvalJobMaxDurationHr},
		{"BacktestEvalJobConcurrencyMin nil", nil, BacktestEvalJobConcurrencyMin, DefaultEvalJobConcurrencyMin},
		{"BacktestEvalJobConcurrencyMax nil", nil, BacktestEvalJobConcurrencyMax, DefaultEvalJobConcurrencyMax},
		{"OpsMigrationsPageCap nil", nil, OpsMigrationsPageCap, DefaultOpsMigrationsPageCap},
		{"OpsRecentMigrationsCount nil", nil, OpsRecentMigrationsCount, DefaultBacktestRecentMigrations},
		{"ResourcesImportMBPerWorker nil", nil, ResourcesImportMBPerWorker, DefaultImportMBPerWorker},
		{"ResourcesExportMBPerWorker nil", nil, ResourcesExportMBPerWorker, DefaultExportMBPerWorker},
		{"ResourcesFieldingMBPerWorker nil", nil, ResourcesFieldingMBPerWorker, DefaultFieldingMBPerWorker},
		{
			"ResourcesMemoryUsageFractionPercent nil",
			nil,
			ResourcesMemoryUsageFractionPercent,
			DefaultMemoryUsageFractionPercent,
		},
		{
			"ResourcesSeqCalcLowMemoryLimitGiB nil",
			nil,
			ResourcesSeqCalcLowMemoryLimitGiB,
			DefaultSeqCalcLowMemoryLimitGiB,
		},
		{
			"ResourcesPrecomputeConcurrencyWhenNoLimit nil",
			nil,
			ResourcesPrecomputeConcurrencyWhenNoLimit,
			DefaultPrecomputeConcurrencyWhenNoLimit,
		},
		{"ServerReadinessTimeoutSec nil", nil, ServerReadinessTimeoutSec, DefaultServerReadinessTimeoutSec},
		{"ResourcesPrecomputeMBPerWorker nil", nil, ResourcesPrecomputeMBPerWorker, DefaultPrecomputeMBPerWorker},
		{
			"SelectionMaxPoolSizeForFullEnum nil",
			nil,
			SelectionMaxPoolSizeForFullEnum,
			DefaultSelectionMaxPoolSizeForFullEnum,
		},
		{
			"SelectionMaxWinProbSwapIterations nil",
			nil,
			SelectionMaxWinProbSwapIterations,
			DefaultSelectionMaxWinProbSwapIterations,
		},
		{"SelectionMaxWinProbEvalBudget nil", nil, SelectionMaxWinProbEvalBudget, DefaultSelectionMaxWinProbEvalBudget},
		{"PipelineReplayMatchPageSize nil", nil, PipelineReplayMatchPageSize, DefaultPipelineReplayMatchPageSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.cfg)
			require.Equal(t, tt.want, got)
		})
	}
}
