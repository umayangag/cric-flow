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

// OpsConfig holds ops dashboard pagination and caps.
type OpsConfig struct {
	MigrationsPageDefault int `json:"migrations_page_default"` // default page size (default 10)
	MigrationsPageMax     int `json:"migrations_page_max"`     // max page size (default 100)
	MigrationsPageCap     int `json:"migrations_page_cap"`     // max page number to avoid huge OFFSET (default 10000)
	RecentMigrationsCount int `json:"recent_migrations_count"` // number of recent migrations for suggestions (default 100)
}

// ResourcesConfig holds the memory estimate and the headroom fraction the import
// pipeline's concurrency is derived from. Import is the only pipeline left that runs
// workers in this process; everything else moved to ml-service (P-5) or was deleted
// with its producer (P-6).
type ResourcesConfig struct {
	ImportMBPerWorker          int `json:"import_mb_per_worker"`          // default 150
	MemoryUsageFractionPercent int `json:"memory_usage_fraction_percent"` // percent of limit for workers (default 85)
}

// PoolConfig bounds the default candidate pool and holds the retirement ledger's
// corroboration thresholds (D-12).
//
// It is configuration rather than a constant because both numbers are measurements of a
// dataset: re-measure on a different one and they move. Both are overridable — the
// window per request, the retirement bounds per deployment.
type PoolConfig struct {
	// RecencyMonths is the default window per format code, in months. A format the map
	// does not name falls back to DefaultPoolRecencyMonths and then to
	// DefaultPoolRecencyMonthsFallback — never to an unbounded pool, which is the defect.
	RecencyMonths map[string]int   `json:"recency_months"`
	Retirement    RetirementConfig `json:"retirement"`
}

// RetirementConfig holds the thresholds the corroboration criteria read.
type RetirementConfig struct {
	// InactiveYears is criterion (a): no match in any format for this many years.
	InactiveYears int `json:"inactive_years"`
	// AgeBoundYears is criterion (c)'s per-format age bound. It has no effect until
	// X-1a supplies dates of birth, and its values are provisional until then.
	AgeBoundYears map[string]int `json:"age_bound_years"`
	// AgeInactiveYears is criterion (c)'s inactivity half: how long a player above the
	// age bound must also have been inactive.
	AgeInactiveYears int `json:"age_inactive_years"`
}

// Config holds directory defaults and the settings go-app still honours.
type Config struct {
	Server ServerConfig `json:"server"`
	Inputs struct {
		CricsheetDir string `json:"cricsheet_dir"`
		// CricsheetSourceURL is the archive Import downloads when the dataset
		// directory does not already hold it. It is configuration rather than a
		// request parameter because a deployment pulls the same archive every time —
		// asking an operator to choose one on every run made acquisition a three-step
		// dance across two tabs (consumer plan W6).
		CricsheetSourceURL string `json:"cricsheet_source_url"`
	} `json:"inputs"`
	Outputs struct {
		// Dir is go-app's own output directory. It held the export CSVs until P-6
		// deleted them; what is left is the resource-observation file the import
		// pipeline's concurrency is derived from.
		Dir string `json:"dir"`
	} `json:"outputs"`
	Formats struct {
		TreatT20ISubset    bool     `json:"treat_t20i_as_subset"`
		InternationalTeams []string `json:"international_teams"`
	} `json:"formats"`
	Team struct {
		MinBowlers     int `json:"min_bowlers"`
		DefaultBatters int `json:"default_batters"`
		DefaultBowlers int `json:"default_bowlers"`
	} `json:"team"`
	Predictor struct {
		TeamSize int `json:"team_size"`
	} `json:"predictor"`
	Ops OpsConfig `json:"ops"`
	// Pool is who may play: the default candidate pool's recency window and the
	// retirement ledger's corroboration thresholds (D-12).
	Pool      PoolConfig       `json:"pool"`
	Resources *ResourcesConfig `json:"resources"` // nil = use package constants
	// Selection is what go-app still decides about an XI. The objective, the search and the
	// weights all moved into ml-service with the XI model (P-5); what is left is the caller's
	// knowledge -- who may play, and how hard the search may work.
	Selection struct {
		MaxWinProbEvalBudget int `json:"max_win_prob_eval_budget"` // XIs ml-service may score per side per round (0 = 500)
		BestResponseRounds   int `json:"best_response_rounds"`     // alternating best-response rounds (0 = 3)
	} `json:"selection"`
	Pipeline struct {
		// ImportTimeoutMs bounds an import run. 0 = no timeout (only shutdown cancels).
		ImportTimeoutMs int `json:"import_timeout_ms"`
		// ImportConcurrency overrides the resource-aware derivation. 0 = auto.
		ImportConcurrency int `json:"import_concurrency"`
	} `json:"pipeline"`
}
