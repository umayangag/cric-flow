// Package config provides application configuration loading and access helpers.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Server holds API/server timeouts, body limits, and URL fallbacks (used when env vars are unset).
type ServerConfig struct {
	MLHealthTimeoutSec     int    `json:"ml_health_timeout_sec"`          // ML health proxy timeout (default 10)
	MLClientTimeoutSec     int    `json:"ml_client_timeout_sec"`          // ML client (predict/train) timeout (default 20)
	MLHealthBodyLimitBytes int    `json:"ml_health_body_limit_bytes"`     // max response size for ML health (default 1048576)
	MLBaseURLFallback      string `json:"ml_base_url_fallback"`           // fallback when ML_SERVICE_URL/ML_BASE_URL unset (default http://localhost:8000)
	ReadinessTimeoutSec    int    `json:"readiness_timeout_sec"`          // DB ping timeout for readiness (default 2)
	TrainStepTimeoutMin    int    `json:"train_step_timeout_min"`         // max wait for ML train endpoint (default 30)
	PipelineProgressSec    int    `json:"pipeline_progress_interval_sec"` // SSE progress poll interval (default 2)
	DBProbeTimeoutSec      int    `json:"db_probe_timeout_sec"`           // ops DB probe timeout (default 2)
	DBProbeLongTimeoutSec  int    `json:"db_probe_long_timeout_sec"`      // ops DB long probe e.g. TableStats (default 5)
	ArtifactsTimeoutSec    int    `json:"artifacts_timeout_sec"`          // HTTP client for artifacts check (default 3)
	HTTPReadTimeoutSec     int    `json:"http_read_timeout_sec"`          // server read timeout (default 15)
	HTTPWriteTimeoutSec    int    `json:"http_write_timeout_sec"`         // server write timeout (default 30)
	HTTPIdleTimeoutSec     int    `json:"http_idle_timeout_sec"`          // server idle timeout (default 60)
	ListenAddress          string `json:"listen_address"`                 // default :8080; PORT env overrides
}

// BacktestJobConfig holds job cleanup and duration for backtest export-contributions and eval jobs.
type BacktestJobConfig struct {
	CleanupAgeHours             int `json:"job_cleanup_age_hours"`                    // remove jobs older than this (default 24)
	CleanupIntervalMin          int `json:"job_cleanup_interval_min"`                 // cleanup run interval (default 15)
	ExportContributionsMaxDurHr int `json:"export_contributions_job_max_duration_hr"` // max duration per job (default 2)
	EvalJobMaxDurationHr        int `json:"eval_job_max_duration_hr"`                 // max duration for eval job (default 6)
	EvalJobConcurrencyMin       int `json:"eval_job_concurrency_min"`                 // min concurrent eval jobs (default 2)
	EvalJobConcurrencyMax       int `json:"eval_job_concurrency_max"`                 // max concurrent eval jobs (default 8)
}

// OpsConfig holds ops dashboard pagination and caps.
type OpsConfig struct {
	MigrationsPageDefault int `json:"migrations_page_default"` // default page size (default 10)
	MigrationsPageMax     int `json:"migrations_page_max"`     // max page size (default 100)
	MigrationsPageCap     int `json:"migrations_page_cap"`     // max page number to avoid huge OFFSET (default 10000)
	RecentMigrationsCount int `json:"recent_migrations_count"` // number of recent migrations for suggestions (default 100)
}

// ResourcesConfig holds memory estimates and concurrency knobs (overridable by env for pipeline kinds).
type ResourcesConfig struct {
	PrecomputeMBPerWorker            int `json:"precompute_mb_per_worker"`             // default 450
	ImportMBPerWorker                int `json:"import_mb_per_worker"`                 // default 150
	ExportMBPerWorker                int `json:"export_mb_per_worker"`                 // default 100
	SeqCalcMBPerWorker               int `json:"seqcalc_mb_per_worker"`                // default 500
	FieldingMBPerWorker              int `json:"fielding_mb_per_worker"`               // default 50
	MemoryUsageFractionPercent       int `json:"memory_usage_fraction_percent"`        // percent of limit for workers (default 80)
	SeqCalcLowMemoryLimitGiB         int `json:"seqcalc_low_memory_limit_gib"`         // cap seqcalc concurrency to 1 below this (default 2)
	PrecomputeConcurrencyWhenNoLimit int `json:"precompute_concurrency_when_no_limit"` // when no GOMEMLIMIT/cgroup (default 2)
}

// Config holds directory defaults for go-app commands.
type Config struct {
	Server ServerConfig `json:"server"`
	Inputs struct {
		CricsheetDir string `json:"cricsheet_dir"`
	} `json:"inputs"`
	Outputs struct {
		ExportDir string `json:"export_dir"`
	} `json:"outputs"`
	Formats struct {
		TreatT20ISubset    bool     `json:"treat_t20i_as_subset"`
		InternationalTeams []string `json:"international_teams"`
	} `json:"formats"`
	Features struct {
		PrecomputeTimeoutMs  int     `json:"precompute_timeout_ms"`
		ExportTimeoutMs      int     `json:"export_timeout_ms"` // optional; 0 = use pipeline timeout
		MinBattingInnings    int     `json:"min_batting_innings"`
		MinBowlingInnings    int     `json:"min_bowling_innings"`
		FormShrinkageAlpha   float32 `json:"form_shrinkage_alpha"`
		ConsistencyPerFormat bool    `json:"consistency_per_format"`
		HistoryWindowMatches int     `json:"history_window_matches"`
		// Feature extraction (EWM form, consistency). Used by export/training-data and precompute when not overridden by CLI.
		EWMAlpha         float64 `json:"ewm_alpha"`          // (0,1]; default 0.3
		EWMAlphaShort    float64 `json:"ewm_alpha_short"`    // for form_short (more recent); default 0.5
		EWMAlphaLong     float64 `json:"ewm_alpha_long"`     // for form_long (longer horizon); default 0.2
		ConsistencyLastN int     `json:"consistency_last_n"` // last-N innings for consistency; default 10
		FormWindowN      int     `json:"form_window_n"`      // max innings for form (0 = no limit); default 0
		MomentumLastN    int     `json:"momentum_last_n"`    // last-N innings for momentum slope; default 5
		FieldingEnrich   struct {
			EWMAlpha           float64 `json:"ewm_alpha"`             // EWM alpha for fielding form fallback; default 0.3
			FormToCatchesRatio float64 `json:"form_to_catches_ratio"` // split of form into catches (rest = run_outs); default 0.7
		} `json:"fielding_enrich"`
	} `json:"features"`
	Export struct {
		SplitByFormat  bool   `json:"split_by_format"`
		RequiredFormat string `json:"required_format"`
	} `json:"export"`
	Team struct {
		MinBowlers     int `json:"min_bowlers"`
		DefaultBatters int `json:"default_batters"`
		DefaultBowlers int `json:"default_bowlers"`
	} `json:"team"`
	Weather struct {
		Enabled           bool     `json:"enabled"`
		RateLimitPerSec   int      `json:"rate_limit_per_sec"`
		MaxAttempts       int      `json:"max_attempts"`
		GeocodeCacheOnly  bool     `json:"geocode_cache_only"`
		OverwriteExisting bool     `json:"overwrite_existing"`
		WhitelistVenues   []string `json:"whitelist_venues"`
		Mocks             struct {
			Temp      int `json:"temp"`
			Wind      int `json:"wind"`
			Rain      int `json:"rain"`
			Humidity  int `json:"humidity"`
			Cloud     int `json:"cloud"`
			Pressure  int `json:"pressure"`
			Viscosity int `json:"viscosity"`
			Session   int `json:"session"`
		} `json:"mocks"`
	} `json:"weather"`
	Predictor struct {
		TeamSize                       int               `json:"team_size"`
		DefaultExtras                  float64           `json:"default_extras"`
		MaxTotalSamples                int               `json:"max_total_samples"`                  // cap on Monte Carlo samples (0 = use default)
		SimulationTopKPerTeam          int               `json:"simulation_top_k_per_team"`          // top XIs per team (0 = use default)
		SimulationNumSamplesPerMatchup int               `json:"simulation_num_samples_per_matchup"` // samples per matchup (0 = use default)
		Simulation                     *SimulationParams `json:"simulation"`                         // CVs for runs/wickets/economy sampling
	} `json:"predictor"`
	Backtest struct {
		ExportMaxMatchIDs         int                `json:"export_max_match_ids"`         // max match_ids per export-contributions request (0 = use default)
		ListDefaultLimit          int                `json:"list_default_limit"`           // default limit for matches-after/holdout (0 = 50)
		ListMaxLimit              int                `json:"list_max_limit"`               // max limit (0 = 500)
		AccuracyTrendDefaultLimit int                `json:"accuracy_trend_default_limit"` // default accuracy-trend limit (0 = 100)
		AccuracyTrendMaxLimit     int                `json:"accuracy_trend_max_limit"`     // max (0 = 500)
		Job                       *BacktestJobConfig `json:"job"`                          // job cleanup/duration; nil = use defaults
	} `json:"backtest"`
	Ops       OpsConfig        `json:"ops"`
	Resources *ResourcesConfig `json:"resources"` // nil = use package constants
	Selection struct {
		DefaultPoolCSV         string                     `json:"default_pool_csv"`
		MaxPoolSizeForFullEnum int                        `json:"max_pool_size_for_full_enum"` // above this use greedy+hill-climb (0 = 18)
		RequireKeeper          bool                       `json:"require_keeper"`
		ScoreWeights           *ScoreWeights              `json:"score_weights"`
		ScoreNormalization     map[string]ScoreNormParams `json:"score_normalization"`
		ScoreWeightsByFormat   map[string]ScoreWeights    `json:"score_weights_by_format"`
		MetaModelPath          string                     `json:"meta_model_path"` // JSON from ml.train_combination_meta
		UseOptimizer           bool                       `json:"use_optimizer"`   // when true, select XI by maximizing total score over valid combinations
	} `json:"selection"`
	// Pipeline optional concurrency overrides (0 = auto from resources package: memory/CPU aware).
	Pipeline struct {
		PrecomputeConcurrency      int `json:"precompute_concurrency"`         // 0 = auto
		ImportConcurrency          int `json:"import_concurrency"`             // 0 = auto (cricsheet)
		SeqCalcConcurrency         int `json:"seqcalc_concurrency"`            // 0 = auto
		ExportConcurrency          int `json:"export_concurrency"`             // 0 = auto
		FieldingConcurrency        int `json:"fielding_concurrency"`           // 0 = auto
		PrecomputeETASecondsPerFmt int `json:"precompute_eta_seconds_per_fmt"` // 0 = use default 180; only used before any format completes; after that ETA uses observed time per format (e.g. set ~2700 for ~45 min per format on slower machines)
		ReplayMatchPageSize        int `json:"replay_match_page_size"`         // matches per chunk in precompute replay (0 = 500)
	} `json:"pipeline"`
}

// SimulationParams holds coefficient-of-variation parameters for Monte Carlo outcome sampling.
type SimulationParams struct {
	RunsCV    float64 `json:"runs_cv"`    // sigma = mean * RunsCV for runs sampling
	WicketsCV float64 `json:"wickets_cv"` // same for wickets
	EconomyCV float64 `json:"economy_cv"` // same for economy
}

// ScoreNormParams holds format-specific divisors for normalizing raw predictions to [0,1].
// BatDivisor: typical max runs per player; runs/divisor caps at 1.
// WicketDivisor: typical max wickets per player.
// EconBase: economy above which contribution is 0; lower economy = higher score.
// FieldDivisor: typical max (catches + run_outs*1.5).
type ScoreNormParams struct {
	BatDivisor    float64 `json:"bat_divisor"`
	WicketDivisor float64 `json:"wicket_divisor"`
	EconBase      float64 `json:"econ_base"`
	FieldDivisor  float64 `json:"field_divisor"`
}

// ScoreWeights defines relative weights for combining batting/bowling/fielding signals in team selection.
type ScoreWeights struct {
	Bat         float64 `json:"bat"`          // default 0.45
	Bowl        float64 `json:"bowl"`         // default 0.40
	Field       float64 `json:"field"`        // default 0.10
	KeeperBonus float64 `json:"keeper_bonus"` // default 0.02
}

var (
	cached     *Config
	loadedFrom string // path of config file loaded; empty if none found
)

// Load reads config.json from the current working directory if present.
// It is safe to call multiple times; the result is cached for the process lifetime.
func Load() *Config {
	if cached != nil {
		return cached
	}
	cfg := &Config{}
	loadedFrom = ""
	if p := os.Getenv("GO_APP_CONFIG"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, cfg)
			cached = cfg
			loadedFrom = p
			return cfg
		}
	}
	candidates := []string{
		"config.json",
		filepath.Join("..", "config.json"),
		filepath.Join("..", "..", "config.json"),
	}
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, cfg)
			cached = cfg
			loadedFrom = p
			return cfg
		}
	}
	cached = cfg
	return cfg
}

// ValidateForServer returns an error if config is missing or invalid for the API server.
// Call at server startup; exit on error rather than using fallback defaults.
func ValidateForServer() error {
	cfg := Load()
	if loadedFrom == "" {
		err := fmt.Errorf("config file not found: set GO_APP_CONFIG or ensure config.json exists (CWD, .., or ../..)")
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	if cfg.Features.PrecomputeTimeoutMs < 0 {
		err := fmt.Errorf(
			"features.precompute_timeout_ms must be >= 0 (0 = no timeout); got %d",
			cfg.Features.PrecomputeTimeoutMs,
		)
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	if cfg.Features.ExportTimeoutMs < 0 {
		err := fmt.Errorf(
			"features.export_timeout_ms must be >= 0 (0 = use pipeline timeout); got %d",
			cfg.Features.ExportTimeoutMs,
		)
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	return nil
}

// PipelineTimeout returns the timeout for long-running pipeline jobs (import, precompute).
// Uses features.precompute_timeout_ms. 0 = no timeout.
func PipelineTimeout() time.Duration {
	cfg := Load()
	if cfg == nil || cfg.Features.PrecomputeTimeoutMs <= 0 {
		return 0
	}
	return time.Duration(cfg.Features.PrecomputeTimeoutMs) * time.Millisecond
}

// ExportTimeout returns the timeout for the export-dataset pipeline step.
// Uses features.export_timeout_ms when set (must be > 0); otherwise PipelineTimeout().
// 0 means no timeout (only shutdown cancels).
func ExportTimeout() time.Duration {
	cfg := Load()
	if cfg != nil && cfg.Features.ExportTimeoutMs > 0 {
		return time.Duration(cfg.Features.ExportTimeoutMs) * time.Millisecond
	}
	return PipelineTimeout()
}

// DefaultCricsheetDir returns the configured cricsheet input dir or a sensible built-in default.
func DefaultCricsheetDir() string {
	cfg := Load()
	if cfg != nil && cfg.Inputs.CricsheetDir != "" {
		return cfg.Inputs.CricsheetDir
	}
	return filepath.Join("..", "data", "go-app", "cricsheet")
}

// EffectiveScoreNormParams returns format-specific normalization divisors for score computation.
// Falls back to defaults when format is not configured.
func EffectiveScoreNormParams(cfg *Config, format string) (batDiv, wicketDiv, econBase, fieldDiv float64) {
	batDiv = DefaultScoreNormBatDivisor
	wicketDiv = DefaultScoreNormWicketDivisor
	econBase = DefaultScoreNormEconBase
	fieldDiv = DefaultScoreNormFieldDivisor
	if cfg != nil && len(cfg.Selection.ScoreNormalization) > 0 {
		if p, ok := cfg.Selection.ScoreNormalization[format]; ok && (p.BatDivisor > 0 || p.WicketDivisor > 0) {
			if p.BatDivisor > 0 {
				batDiv = p.BatDivisor
			}
			if p.WicketDivisor > 0 {
				wicketDiv = p.WicketDivisor
			}
			if p.EconBase > 0 {
				econBase = p.EconBase
			}
			if p.FieldDivisor > 0 {
				fieldDiv = p.FieldDivisor
			}
		}
	}
	return batDiv, wicketDiv, econBase, fieldDiv
}

// metaModelWeights holds parsed coefficients from train_combination_meta JSON.
type metaModelWeights struct {
	Bat         float64                     `json:"bat"`
	Bowl        float64                     `json:"bowl"`
	Field       float64                     `json:"field"`
	KeeperBonus float64                     `json:"keeper_bonus"`
	PerFormat   map[string]metaModelWeights `json:"per_format"`
}

var (
	metaModelCache  *metaModelWeights
	metaModelPath   string
	metaModelLoadMu sync.Mutex
)

func loadMetaModel(cfg *Config) *metaModelWeights {
	path := ""
	if cfg != nil && cfg.Selection.MetaModelPath != "" {
		path = cfg.Selection.MetaModelPath
	}
	if path == "" {
		return nil
	}
	metaModelLoadMu.Lock()
	defer metaModelLoadMu.Unlock()
	if metaModelPath == path && metaModelCache != nil {
		return metaModelCache
	}
	abs := path
	if !filepath.IsAbs(path) {
		// If a config file was loaded, resolve relative to its directory.
		if loadedFrom != "" {
			configDir := filepath.Dir(loadedFrom)
			abs = filepath.Join(configDir, path)
		} else {
			// Fallback to CWD if config path is unknown (e.g. in tests)
			cwd, err := os.Getwd()
			if err != nil {
				slog.Error("config.loadMetaModel failed to get CWD", "err", err)
				return nil
			}
			abs = filepath.Join(cwd, path)
		}
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		slog.Error("config.loadMetaModel failed to read file", "path", abs, "err", err)
		metaModelPath = ""
		metaModelCache = nil
		return nil
	}
	var m metaModelWeights
	if err := json.Unmarshal(b, &m); err != nil {
		slog.Error("config.loadMetaModel unmarshal failed", "path", abs, "err", err)
		metaModelPath = ""
		metaModelCache = nil
		return nil
	}
	metaModelPath = path
	metaModelCache = &m
	return metaModelCache
}

// EffectiveScoreWeightsForFormat returns score weights for the given format, with per-format override when configured.
func EffectiveScoreWeightsForFormat(cfg *Config, format string) (bat, bowl, field, keeperBonus float64) {
	// 1. Meta-model (learned from backtest) takes precedence when configured
	if meta := loadMetaModel(cfg); meta != nil {
		if len(meta.PerFormat) > 0 {
			if w, ok := meta.PerFormat[format]; ok {
				return w.Bat, w.Bowl, w.Field, w.KeeperBonus
			}
		}
		if meta.Bat > 0 || meta.Bowl > 0 {
			return meta.Bat, meta.Bowl, meta.Field, meta.KeeperBonus
		}
	}
	// 2. Config score_weights_by_format
	if cfg != nil && len(cfg.Selection.ScoreWeightsByFormat) > 0 {
		if w, ok := cfg.Selection.ScoreWeightsByFormat[format]; ok {
			return w.Bat, w.Bowl, w.Field, w.KeeperBonus
		}
	}
	return EffectiveScoreWeights(cfg)
}

// EffectiveScoreWeights returns the configured score weights or built-in defaults.
func EffectiveScoreWeights(cfg *Config) (bat, bowl, field, keeperBonus float64) {
	if cfg != nil && cfg.Selection.ScoreWeights != nil {
		w := cfg.Selection.ScoreWeights
		bat = w.Bat
		bowl = w.Bowl
		field = w.Field
		keeperBonus = w.KeeperBonus
	}
	if bat == 0 {
		bat = DefaultScoreWeightBat
	}
	if bowl == 0 {
		bowl = DefaultScoreWeightBowl
	}
	if field == 0 {
		field = DefaultScoreWeightField
	}
	if keeperBonus == 0 {
		keeperBonus = DefaultScoreWeightKeeperBonus
	}
	return bat, bowl, field, keeperBonus
}

// EffectiveExportMaxMatchIDs returns the backtest export-contributions match_ids limit.
func EffectiveExportMaxMatchIDs(cfg *Config) int {
	if cfg != nil && cfg.Backtest.ExportMaxMatchIDs > 0 {
		return cfg.Backtest.ExportMaxMatchIDs
	}
	return DefaultExportMaxMatchIDs
}

// EffectiveMaxTotalSamples returns the cap on Monte Carlo total samples (matchups * samples per matchup).
func EffectiveMaxTotalSamples(cfg *Config) int {
	if cfg != nil && cfg.Predictor.MaxTotalSamples > 0 {
		return cfg.Predictor.MaxTotalSamples
	}
	return DefaultMaxTotalSamples
}

// EffectiveSimulationTopKPerTeam returns the default top-k XIs per team for simulation (0 = use default).
func EffectiveSimulationTopKPerTeam(cfg *Config) int {
	if cfg != nil && cfg.Predictor.SimulationTopKPerTeam > 0 {
		return cfg.Predictor.SimulationTopKPerTeam
	}
	return DefaultSimulationTopKPerTeam
}

// EffectiveSimulationNumSamplesPerMatchup returns the default samples per matchup for simulation (0 = use default).
func EffectiveSimulationNumSamplesPerMatchup(cfg *Config) int {
	if cfg != nil && cfg.Predictor.SimulationNumSamplesPerMatchup > 0 {
		return cfg.Predictor.SimulationNumSamplesPerMatchup
	}
	return DefaultSimulationNumSamplesPerMatchup
}

// EffectiveSimulationCVs returns RunsCV, WicketsCV, EconomyCV for Monte Carlo sampling (0 = use default).
func EffectiveSimulationCVs(cfg *Config) (runsCV, wicketsCV, economyCV float64) {
	runsCV = DefaultSimulationRunsCV
	wicketsCV = DefaultSimulationWicketsCV
	economyCV = DefaultSimulationEconomyCV
	if cfg != nil && cfg.Predictor.Simulation != nil {
		s := cfg.Predictor.Simulation
		if s.RunsCV > 0 {
			runsCV = s.RunsCV
		}
		if s.WicketsCV > 0 {
			wicketsCV = s.WicketsCV
		}
		if s.EconomyCV > 0 {
			economyCV = s.EconomyCV
		}
	}
	return runsCV, wicketsCV, economyCV
}

// Server helpers (timeouts in seconds/minutes; 0 = use default from constants).
func ServerMLHealthTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.MLHealthTimeoutSec > 0 {
		return cfg.Server.MLHealthTimeoutSec
	}
	return DefaultServerMLHealthTimeoutSec
}

func ServerMLClientTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.MLClientTimeoutSec > 0 {
		return cfg.Server.MLClientTimeoutSec
	}
	return DefaultServerMLClientTimeoutSec
}

func ServerMLHealthBodyLimitBytes(cfg *Config) int {
	if cfg != nil && cfg.Server.MLHealthBodyLimitBytes > 0 {
		return cfg.Server.MLHealthBodyLimitBytes
	}
	return DefaultServerMLHealthBodyLimitBytes
}

func ServerMLBaseURLFallback(cfg *Config) string {
	if cfg != nil && cfg.Server.MLBaseURLFallback != "" {
		return cfg.Server.MLBaseURLFallback
	}
	return "http://localhost:8000"
}

func ServerReadinessTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.ReadinessTimeoutSec > 0 {
		return cfg.Server.ReadinessTimeoutSec
	}
	return DefaultServerReadinessTimeoutSec
}

func ServerTrainStepTimeoutMin(cfg *Config) int {
	if cfg != nil && cfg.Server.TrainStepTimeoutMin > 0 {
		return cfg.Server.TrainStepTimeoutMin
	}
	return DefaultServerTrainStepTimeoutMin
}

func ServerPipelineProgressSec(cfg *Config) int {
	if cfg != nil && cfg.Server.PipelineProgressSec > 0 {
		return cfg.Server.PipelineProgressSec
	}
	return DefaultServerPipelineProgressSec
}

func ServerDBProbeTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.DBProbeTimeoutSec > 0 {
		return cfg.Server.DBProbeTimeoutSec
	}
	return DefaultServerDBProbeTimeoutSec
}

func ServerDBProbeLongTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.DBProbeLongTimeoutSec > 0 {
		return cfg.Server.DBProbeLongTimeoutSec
	}
	return DefaultServerDBProbeLongTimeoutSec
}

func ServerArtifactsTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.ArtifactsTimeoutSec > 0 {
		return cfg.Server.ArtifactsTimeoutSec
	}
	return DefaultServerArtifactsTimeoutSec
}

func ServerHTTPReadTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.HTTPReadTimeoutSec > 0 {
		return cfg.Server.HTTPReadTimeoutSec
	}
	return DefaultServerHTTPReadTimeoutSec
}

func ServerHTTPWriteTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.HTTPWriteTimeoutSec > 0 {
		return cfg.Server.HTTPWriteTimeoutSec
	}
	return DefaultServerHTTPWriteTimeoutSec
}

func ServerHTTPIdleTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.HTTPIdleTimeoutSec > 0 {
		return cfg.Server.HTTPIdleTimeoutSec
	}
	return DefaultServerHTTPIdleTimeoutSec
}

func ServerListenAddress(cfg *Config) string {
	if cfg != nil && cfg.Server.ListenAddress != "" {
		return cfg.Server.ListenAddress
	}
	return DefaultServerListenAddress
}

// Pipeline precompute ETA seconds per format (used before any format completes).
func PipelinePrecomputeETASecondsPerFmt(cfg *Config) int {
	if cfg != nil && cfg.Pipeline.PrecomputeETASecondsPerFmt > 0 {
		return cfg.Pipeline.PrecomputeETASecondsPerFmt
	}
	return DefaultServerPrecomputeETASecPerFmt
}

// Backtest list/accuracy limits and job config.
func BacktestListDefaultLimit(cfg *Config) int {
	if cfg != nil && cfg.Backtest.ListDefaultLimit > 0 {
		return cfg.Backtest.ListDefaultLimit
	}
	return DefaultBacktestListDefaultLimit
}

func BacktestListMaxLimit(cfg *Config) int {
	if cfg != nil && cfg.Backtest.ListMaxLimit > 0 {
		return cfg.Backtest.ListMaxLimit
	}
	return DefaultBacktestListMaxLimit
}

func BacktestAccuracyTrendDefaultLimit(cfg *Config) int {
	if cfg != nil && cfg.Backtest.AccuracyTrendDefaultLimit > 0 {
		return cfg.Backtest.AccuracyTrendDefaultLimit
	}
	return DefaultBacktestAccuracyTrendLimit
}

func BacktestAccuracyTrendMaxLimit(cfg *Config) int {
	if cfg != nil && cfg.Backtest.AccuracyTrendMaxLimit > 0 {
		return cfg.Backtest.AccuracyTrendMaxLimit
	}
	return DefaultBacktestListMaxLimit
}

func BacktestJobCleanupAgeHours(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.CleanupAgeHours > 0 {
		return cfg.Backtest.Job.CleanupAgeHours
	}
	return DefaultBacktestJobCleanupAgeHours
}

func BacktestJobCleanupIntervalMin(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.CleanupIntervalMin > 0 {
		return cfg.Backtest.Job.CleanupIntervalMin
	}
	return DefaultBacktestJobCleanupIntervalMin
}

func BacktestExportContributionsJobMaxDurationHr(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.ExportContributionsMaxDurHr > 0 {
		return cfg.Backtest.Job.ExportContributionsMaxDurHr
	}
	return DefaultExportContributionsJobMaxDurHr
}

func BacktestEvalJobMaxDurationHr(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.EvalJobMaxDurationHr > 0 {
		return cfg.Backtest.Job.EvalJobMaxDurationHr
	}
	return DefaultEvalJobMaxDurationHr
}

func BacktestEvalJobConcurrencyMin(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.EvalJobConcurrencyMin > 0 {
		return cfg.Backtest.Job.EvalJobConcurrencyMin
	}
	return DefaultEvalJobConcurrencyMin
}

func BacktestEvalJobConcurrencyMax(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.EvalJobConcurrencyMax > 0 {
		return cfg.Backtest.Job.EvalJobConcurrencyMax
	}
	return DefaultEvalJobConcurrencyMax
}

// Ops pagination.
func OpsMigrationsPageDefault(cfg *Config) int {
	if cfg != nil && cfg.Ops.MigrationsPageDefault > 0 {
		return cfg.Ops.MigrationsPageDefault
	}
	return DefaultOpsMigrationsPageDefault
}

func OpsMigrationsPageMax(cfg *Config) int {
	if cfg != nil && cfg.Ops.MigrationsPageMax > 0 {
		return cfg.Ops.MigrationsPageMax
	}
	return DefaultOpsMigrationsPageMax
}

func OpsMigrationsPageCap(cfg *Config) int {
	if cfg != nil && cfg.Ops.MigrationsPageCap > 0 {
		return cfg.Ops.MigrationsPageCap
	}
	return DefaultOpsMigrationsPageCap
}

func OpsRecentMigrationsCount(cfg *Config) int {
	if cfg != nil && cfg.Ops.RecentMigrationsCount > 0 {
		return cfg.Ops.RecentMigrationsCount
	}
	return DefaultBacktestRecentMigrations
}

// Pipeline replay page size.
func PipelineReplayMatchPageSize(cfg *Config) int {
	if cfg != nil && cfg.Pipeline.ReplayMatchPageSize > 0 {
		return cfg.Pipeline.ReplayMatchPageSize
	}
	return DefaultPipelineReplayMatchPageSize
}

// Selection max pool size for full enumeration.
func SelectionMaxPoolSizeForFullEnum(cfg *Config) int {
	if cfg != nil && cfg.Selection.MaxPoolSizeForFullEnum > 0 {
		return cfg.Selection.MaxPoolSizeForFullEnum
	}
	return DefaultSelectionMaxPoolSizeForFullEnum
}

// Resource limits (used by resources package). 0 in config = use default constant.
func ResourcesPrecomputeMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.PrecomputeMBPerWorker > 0 {
		return cfg.Resources.PrecomputeMBPerWorker
	}
	return DefaultPrecomputeMBPerWorker
}

func ResourcesImportMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.ImportMBPerWorker > 0 {
		return cfg.Resources.ImportMBPerWorker
	}
	return DefaultImportMBPerWorker
}

func ResourcesExportMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.ExportMBPerWorker > 0 {
		return cfg.Resources.ExportMBPerWorker
	}
	return DefaultExportMBPerWorker
}

func ResourcesSeqCalcMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.SeqCalcMBPerWorker > 0 {
		return cfg.Resources.SeqCalcMBPerWorker
	}
	return DefaultSeqCalcMBPerWorker
}

func ResourcesFieldingMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.FieldingMBPerWorker > 0 {
		return cfg.Resources.FieldingMBPerWorker
	}
	return DefaultFieldingMBPerWorker
}

func ResourcesMemoryUsageFractionPercent(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.MemoryUsageFractionPercent > 0 {
		return cfg.Resources.MemoryUsageFractionPercent
	}
	return DefaultMemoryUsageFractionPercent
}

func ResourcesSeqCalcLowMemoryLimitGiB(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.SeqCalcLowMemoryLimitGiB > 0 {
		return cfg.Resources.SeqCalcLowMemoryLimitGiB
	}
	return DefaultSeqCalcLowMemoryLimitGiB
}

func ResourcesPrecomputeConcurrencyWhenNoLimit(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.PrecomputeConcurrencyWhenNoLimit > 0 {
		return cfg.Resources.PrecomputeConcurrencyWhenNoLimit
	}
	return DefaultPrecomputeConcurrencyWhenNoLimit
}

// DefaultExportDir returns the configured export output dir or a built-in default.
// GO_APP_OUTPUT_DIR (when set) overrides config so the API can use a writable path in Docker.
// Otherwise config or cwd-relative "output/go-app" is used.
func DefaultExportDir() string {
	if p := os.Getenv("GO_APP_OUTPUT_DIR"); p != "" {
		return p
	}
	cfg := Load()
	if cfg != nil && cfg.Outputs.ExportDir != "" {
		return cfg.Outputs.ExportDir
	}
	return filepath.Join("output", "go-app")
}

// ValidateTeamSettings validates a subset of team/predictor settings for sanity.
// It is a pure helper and does not perform any I/O. It does not modify cfg.
// Only non-zero values are validated for cross-field constraints to avoid
// over-constraining partially-specified configs. Callers may enforce stricter
// policies as needed at the edges (e.g., in main).
func ValidateTeamSettings(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	// Min bowlers must be at least 1.
	if cfg.Team.MinBowlers < 1 {
		err := fmt.Errorf("min bowlers must be >= 1")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// Default extras cannot be negative when provided.
	if cfg.Predictor.DefaultExtras < 0 {
		err := fmt.Errorf("extras must be non-negative")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// Default batters cannot be negative.
	if cfg.Team.DefaultBatters < 0 {
		err := fmt.Errorf("default batters must be >= 0")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// If both TeamSize and MinBowlers are provided, TeamSize must be >= MinBowlers.
	if cfg.Predictor.TeamSize > 0 && cfg.Team.MinBowlers > 0 && cfg.Predictor.TeamSize < cfg.Team.MinBowlers {
		err := fmt.Errorf("team size must be >= min bowlers")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// If DefaultBowlers is set, it must be >= MinBowlers.
	if cfg.Team.DefaultBowlers > 0 && cfg.Team.MinBowlers > 0 && cfg.Team.DefaultBowlers < cfg.Team.MinBowlers {
		err := fmt.Errorf("default bowlers must be >= min bowlers")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	return nil
}
