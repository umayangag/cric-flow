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
	testCases := []struct {
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

	for i := range testCases {
		tc := testCases[i]
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

	testCases := []struct {
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
		{
			"BacktestAccuracyTrendConcurrency nil",
			nil,
			BacktestAccuracyTrendConcurrency,
			DefaultBacktestAccuracyTrendConcurrency,
		},
		{
			"BacktestAccuracyTrendConcurrency set",
			func() *Config { c := &Config{}; c.Backtest.AccuracyTrendConcurrency = 4; return c }(),
			BacktestAccuracyTrendConcurrency,
			4,
		},
		{
			"BacktestExportContributionsConcurrency nil",
			nil,
			BacktestExportContributionsConcurrency,
			DefaultBacktestExportContributionsConcurrency,
		},
		{
			"BacktestExportContributionsConcurrency set",
			func() *Config { c := &Config{}; c.Backtest.ExportContributionsConcurrency = 6; return c }(),
			BacktestExportContributionsConcurrency,
			6,
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
		{"SelectionMaxWinProbEvalBudget nil", nil, SelectionMaxWinProbEvalBudget, DefaultSelectionMaxWinProbEvalBudget},
		{"PipelineReplayMatchPageSize nil", nil, PipelineReplayMatchPageSize, DefaultPipelineReplayMatchPageSize},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn(tc.cfg)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestCricsheetSourceURL covers the setting Import acquires from (consumer plan W6-1).
//
// The default matters as much as the override: the point of W6 is that Import works
// on a box nobody has configured, so an empty or absent setting must not mean "no
// source", it must mean "the usual one".
func TestCricsheetSourceURL(t *testing.T) {
	// Not parallel: mutates the package-level cached config, like the tests above.
	t.Run("falls back to the built-in default", func(t *testing.T) {
		cached = &Config{}
		require.Equal(t, DefaultCricsheetSourceURL, CricsheetSourceURL())
	})

	t.Run("uses the configured URL", func(t *testing.T) {
		cached = &Config{}
		cached.Inputs.CricsheetSourceURL = "https://cricsheet.org/downloads/t20s_json.zip"
		require.Equal(t, "https://cricsheet.org/downloads/t20s_json.zip", CricsheetSourceURL())
	})

	t.Run("treats a blank setting as unset rather than as no source", func(t *testing.T) {
		cached = &Config{}
		cached.Inputs.CricsheetSourceURL = "   "
		require.Equal(t, DefaultCricsheetSourceURL, CricsheetSourceURL())
	})
}

// TestRetiredKeys covers the P-5 rule that a setting whose behaviour was deleted must fail
// validation naming the PR, not be silently ignored (the retired-parameter rule, applied to
// config: encoding/json drops a key with no field, so nothing else would ever notice).
func TestRetiredKeys(t *testing.T) {
	testCases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "no config bytes", raw: "", want: ""},
		{name: "unparseable config is the loader's problem", raw: "{not json", want: ""},
		{name: "a config with nothing retired", raw: `{"selection":{"best_response_rounds":3}}`, want: ""},
		{
			name: "the stale win_model key names the PR that removed it",
			raw:  `{"selection":{"win_model":"xi"}}`,
			want: "selection.win_model (removed in P-5",
		},
		{
			name: "a retired key set to null is still an intention",
			raw:  `{"selection":{"score_weights":null}}`,
			want: "selection.score_weights (removed in P-5",
		},
		{
			name: "a retired key under a different parent is not matched",
			raw:  `{"backtest":{"win_model":"xi"}}`,
			want: "",
		},
		{
			name: "every retired key present is reported at once",
			raw:  `{"selection":{"win_model":"xi","use_optimizer":true},"predictor":{"simulation":{}}}`,
			want: "3 retired setting(s)",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := RetiredKeys([]byte(tc.raw))
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
