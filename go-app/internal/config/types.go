package config

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
	FieldingMBPerWorker              int `json:"fielding_mb_per_worker"`               // default 100
	MemoryUsageFractionPercent       int `json:"memory_usage_fraction_percent"`        // percent of limit for workers (default 80)
	SeqCalcLowMemoryLimitGiB         int `json:"seqcalc_low_memory_limit_gib"`         // cap seqcalc concurrency to 1 below this (default 2)
	PrecomputeConcurrencyWhenNoLimit int `json:"precompute_concurrency_when_no_limit"` // 0 = auto (NumCPU); set >0 to cap (default 0)
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
		ExportMaxMatchIDs         int                `json:"export_max_match_ids"`          // max match_ids per export-contributions request (0 = use default)
		ListDefaultLimit          int                `json:"list_default_limit"`            // default limit for matches-after/holdout (0 = 50)
		ListMaxLimit              int                `json:"list_max_limit"`                // max limit (0 = 500)
		AccuracyTrendDefaultLimit int                `json:"accuracy_trend_default_limit"`  // default accuracy-trend limit (0 = 100)
		AccuracyTrendMaxLimit     int                `json:"accuracy_trend_max_limit"`      // max (0 = 500)
		AccuracyTrendConcurrency  int                `json:"accuracy_trend_concurrency"`    // max concurrent workers for accuracy-trend computations (0 = use default)
		Job                       *BacktestJobConfig `json:"job"`                           // job cleanup/duration; nil = use defaults
	} `json:"backtest"`
	Ops       OpsConfig        `json:"ops"`
	Resources *ResourcesConfig `json:"resources"` // nil = use package constants
	Selection struct {
		DefaultPoolCSV             string                     `json:"default_pool_csv"`
		MaxPoolSizeForFullEnum     int                        `json:"max_pool_size_for_full_enum"` // above this use greedy+hill-climb (0 = 18)
		RequireKeeper              bool                       `json:"require_keeper"`
		ScoreWeights               *ScoreWeights              `json:"score_weights"`
		ScoreNormalization         map[string]ScoreNormParams `json:"score_normalization"`
		ScoreWeightsByFormat       map[string]ScoreWeights    `json:"score_weights_by_format"`
		MetaModelPath              string                     `json:"meta_model_path"`               // JSON from ml.train_combination_meta
		UseOptimizer               bool                       `json:"use_optimizer"`                 // when true, select XI by maximizing total score over valid combinations
		UseWinProbabilitySelection bool                       `json:"use_win_probability_selection"` // when true, select XI by maximizing win probability via hill-climb
		MaxWinProbSwapIterations   int                        `json:"max_win_prob_swap_iterations"`  // hill-climb outer-loop cap for win-prob selection (0 = 50)
		MaxWinProbEvalBudget       int                        `json:"max_win_prob_eval_budget"`      // total ML eval calls allowed per team in win-prob hill-climb (0 = 500)
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

// metaModelWeights holds parsed coefficients from train_combination_meta JSON.
type metaModelWeights struct {
	Bat         float64                     `json:"bat"`
	Bowl        float64                     `json:"bowl"`
	Field       float64                     `json:"field"`
	KeeperBonus float64                     `json:"keeper_bonus"`
	PerFormat   map[string]metaModelWeights `json:"per_format"`
}
