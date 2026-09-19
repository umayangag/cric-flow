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
				return c
			}(),
			err: "",
		},
		{
			name: "min bowlers < 1",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 0
				return c
			}(),
			err: "min bowlers",
		},
		{
			name: "min bowlers above the eleven",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 12
				return c
			}(),
			err: "min bowlers must be <= 11",
		},
		{
			name: "default bowlers < min bowlers",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 5
				c.Team.DefaultBowlers = 3
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
	cached.Outputs.Dir = "/tmp/export"

	require.Equal(t, "/tmp/cricsheet", DefaultCricsheetDir())
	require.Equal(t, "/tmp/export", DefaultOutputDir())
}

// Not parallel: uses and mutates the package-level cached config.
func TestDefaultDirs_FallbacksWhenUnset(t *testing.T) {
	cached = &Config{} // simulate empty config loaded
	wantCricsheet := filepath.Join("..", "data", "go-app", "cricsheet")
	wantExport := filepath.Join("output", "go-app")

	require.Equal(t, wantCricsheet, DefaultCricsheetDir())
	require.Equal(t, wantExport, DefaultOutputDir())
}

// Not parallel: sets env and mutates cached config.
func TestDefaultOutputDir_EnvOverridesConfig(t *testing.T) {
	cached = &Config{}
	cached.Outputs.Dir = "/tmp/export"
	t.Setenv("GO_APP_OUTPUT_DIR", "/output/go-app")
	defer t.Setenv("GO_APP_OUTPUT_DIR", "")

	require.Equal(t, "/output/go-app", DefaultOutputDir())
}

func TestPipelineTimeout(t *testing.T) {
	cached = nil
	cached = &Config{}
	defer func() { cached = nil }()

	// no timeout configured
	d := PipelineTimeout()
	require.Equal(t, time.Duration(0), d)

	// configured timeout
	cached.Pipeline.ImportTimeoutMs = 60000
	d = PipelineTimeout()
	require.Equal(t, 60*time.Second, d)
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

func TestConfigServerAndResourcesHelpers(t *testing.T) {
	cfg := &Config{}
	cfg.Server.ReadinessTimeoutSec = 5
	require.Equal(t, 5, ServerReadinessTimeoutSec(cfg))

	cfg.Resources = &ResourcesConfig{ImportMBPerWorker: 600}
	require.Equal(t, 600, ResourcesImportMBPerWorker(cfg))

	cfg.Selection.MaxWinProbEvalBudget = 200
	require.Equal(t, 200, SelectionMaxWinProbEvalBudget(cfg))

	require.Equal(t, DefaultOpsMigrationsPageDefault, OpsMigrationsPageDefault(nil))
	cfg.Ops.MigrationsPageDefault = 20
	require.Equal(t, 20, OpsMigrationsPageDefault(cfg))
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

func TestConfigMoreServerHelpers(t *testing.T) {
	cfgServerTimeouts := &Config{}
	cfgServerTimeouts.Server.DBProbeLongTimeoutSec = 10
	cfgServerTimeouts.Server.ArtifactsTimeoutSec = 120
	cfgServerTimeouts.Server.HTTPReadTimeoutSec = 15
	cfgServerTimeouts.Server.HTTPWriteTimeoutSec = 20
	cfgServerTimeouts.Server.HTTPIdleTimeoutSec = 25

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
		{"ServerDBProbeLongTimeoutSec set", cfgServerTimeouts, ServerDBProbeLongTimeoutSec, 10},
		{"ServerArtifactsTimeoutSec set", cfgServerTimeouts, ServerArtifactsTimeoutSec, 120},
		{"ServerHTTPReadTimeoutSec set", cfgServerTimeouts, ServerHTTPReadTimeoutSec, 15},
		{"ServerHTTPWriteTimeoutSec set", cfgServerTimeouts, ServerHTTPWriteTimeoutSec, 20},
		{"ServerHTTPIdleTimeoutSec nil", nil, ServerHTTPIdleTimeoutSec, DefaultServerHTTPIdleTimeoutSec},
		{"ServerHTTPIdleTimeoutSec set", cfgServerTimeouts, ServerHTTPIdleTimeoutSec, 25},
		{"OpsMigrationsPageMax nil", nil, OpsMigrationsPageMax, DefaultOpsMigrationsPageMax},
		{
			"OpsMigrationsPageMax set",
			&Config{Ops: OpsConfig{MigrationsPageMax: 500}},
			OpsMigrationsPageMax,
			500,
		},
		{"OpsMigrationsPageCap nil", nil, OpsMigrationsPageCap, DefaultOpsMigrationsPageCap},
		{"OpsMigrationsPageCap set", &Config{Ops: OpsConfig{MigrationsPageCap: 5000}}, OpsMigrationsPageCap, 5000},
		{"OpsRecentMigrationsCount nil", nil, OpsRecentMigrationsCount, DefaultRecentMigrations},
		{"ResourcesImportMBPerWorker nil", nil, ResourcesImportMBPerWorker, DefaultImportMBPerWorker},
		{
			"ResourcesMemoryUsageFractionPercent nil",
			nil,
			ResourcesMemoryUsageFractionPercent,
			DefaultMemoryUsageFractionPercent,
		},
		{"ServerReadinessTimeoutSec nil", nil, ServerReadinessTimeoutSec, DefaultServerReadinessTimeoutSec},
		{"SelectionMaxWinProbEvalBudget nil", nil, SelectionMaxWinProbEvalBudget, DefaultSelectionMaxWinProbEvalBudget},
		{"SelectionBestResponseRounds nil", nil, SelectionBestResponseRounds, DefaultSelectionBestResponseRounds},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn(tc.cfg)
			require.Equal(t, tc.want, got)
		})
	}
}

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
			raw:  `{"team":{"win_model":"xi"}}`,
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
