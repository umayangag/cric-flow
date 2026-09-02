package config

import "time"

// DefaultTimeout is the default timeout for long-running CLI operations (imports, precompute, etc.).
// Set to 1 year so runs effectively have no deadline; use -timeout to cap (e.g. -timeout=5h).
const DefaultTimeout = 7 * 24 * time.Hour

// Feature extraction defaults used when config is missing or values are zero.
// These affect prediction results; keep in sync with config.json defaults.
const (
	DefaultFeatureEWMAlpha         = 0.3
	DefaultFeatureEWMAlphaShort    = 0.5 // more weight to recent for form_short
	DefaultFeatureEWMAlphaLong     = 0.2 // less weight to recent for form_long
	DefaultFeatureConsistencyLastN = 10
	DefaultFeatureFormWindowN      = 0
	DefaultFeatureMomentumLastN    = 5
)

// Fielding enrich defaults (used when ML has no fielding model; go-app enriches from history).
const (
	DefaultFieldingEWMAlpha           = 0.3
	DefaultFieldingFormToCatchesRatio = 0.7
)

// Team selection defaults.
const (
	DefaultMinBowlers     = 5
	DefaultDefaultBatters = 6
	DefaultDefaultBowlers = 5
	DefaultTeamSize       = 11
)

// Backtest / export-contributions defaults.
const (
	DefaultExportMaxMatchIDs = 200
)

// Server/API defaults (timeouts, body limits, pagination). Used when server config is missing or zero.
const (
	DefaultServerMLHealthTimeoutSec     = 10
	DefaultServerMLClientTimeoutSec     = 20
	DefaultServerMLHealthBodyLimitBytes = 1 << 20 // 1 MiB
	DefaultServerReadinessTimeoutSec    = 2
	DefaultServerTrainStepTimeoutMin    = 30
	DefaultServerPipelineProgressSec    = 2
	DefaultServerPrecomputeETASecPerFmt = 180
	DefaultServerDBProbeTimeoutSec      = 2
	DefaultServerDBProbeLongTimeoutSec  = 5
	DefaultServerArtifactsTimeoutSec    = 3
	DefaultServerHTTPReadTimeoutSec     = 15
	DefaultServerHTTPWriteTimeoutSec    = 30
	DefaultServerHTTPIdleTimeoutSec     = 60
	DefaultServerListenAddress          = ":8080"
)

// Backtest list/holdout and accuracy-trend defaults and caps.
const (
	DefaultBacktestListDefaultLimit               = 50
	DefaultBacktestListMaxLimit                   = 500
	DefaultBacktestAccuracyTrendLimit             = 100
	DefaultBacktestRecentMigrations               = 100
	DefaultBacktestAccuracyTrendConcurrency       = 8
	DefaultBacktestExportContributionsConcurrency = 8
)

// Backtest job cleanup and duration (export-contributions and eval jobs).
const (
	DefaultBacktestJobCleanupAgeHours     = 24
	DefaultBacktestJobCleanupIntervalMin  = 15
	DefaultExportContributionsJobMaxDurHr = 2
	DefaultEvalJobMaxDurationHr           = 6
	DefaultEvalJobConcurrencyMin          = 2
	DefaultEvalJobConcurrencyMax          = 8
)

// Ops pagination and caps.
const (
	DefaultOpsMigrationsPageDefault = 10
	DefaultOpsMigrationsPageMax     = 100
	DefaultOpsMigrationsPageCap     = 10000
)

// Pipeline precompute replay chunk size (matches per page to limit memory).
const DefaultPipelineReplayMatchPageSize = 500

// Resources: memory per worker (MB), fraction of limit for workers (percent), seqcalc low-memory threshold (GiB), default precompute concurrency when no limit.
const (
	DefaultPrecomputeMBPerWorker = 450
	DefaultImportMBPerWorker     = 150
	DefaultExportMBPerWorker     = 100
	DefaultSeqCalcMBPerWorker    = 500
	DefaultFieldingMBPerWorker   = 100
	// DefaultMemoryUsageFractionPercent: fraction of container memory used for workers (rest for Go runtime, DB, spikes).
	// Set to 85 to provide more headroom for the Go runtime and database, especially in memory-constrained environments.
	// A lower value may slightly reduce throughput but increases stability by reducing OOM risk.
	DefaultMemoryUsageFractionPercent = 85
	DefaultSeqCalcLowMemoryLimitGiB   = 2
	// 0 = use NumCPU() when no memory limit (optimum resources); set > 0 to cap precompute concurrency.
	DefaultPrecomputeConcurrencyWhenNoLimit = 0
)

// Win-probability selection: how hard ml-service may search, and for how many rounds.
const (
	DefaultSelectionMaxWinProbEvalBudget = 500
	// DefaultSelectionBestResponseRounds caps the alternating best-response rounds in
	// win-probability selection. Best response can cycle rather than converge, so the
	// loop is bounded; three rounds is enough for the fixed point when there is one.
	DefaultSelectionBestResponseRounds = 3
)
