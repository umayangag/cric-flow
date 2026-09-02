package config

import "time"

// DefaultTimeout is the default timeout for long-running CLI operations (imports).
// Set to 1 year so runs effectively have no deadline; use -timeout to cap (e.g. -timeout=5h).
const DefaultTimeout = 7 * 24 * time.Hour

// Team selection defaults.
const (
	DefaultMinBowlers     = 5
	DefaultDefaultBatters = 6
	DefaultDefaultBowlers = 5
	DefaultTeamSize       = 11
)

// Server/API defaults (timeouts, body limits, pagination). Used when server config is missing or zero.
const (
	DefaultServerMLHealthTimeoutSec     = 10
	DefaultServerMLClientTimeoutSec     = 20
	DefaultServerMLHealthBodyLimitBytes = 1 << 20 // 1 MiB
	DefaultServerReadinessTimeoutSec    = 2
	DefaultServerTrainStepTimeoutMin    = 30
	DefaultServerPipelineProgressSec    = 2
	DefaultServerDBProbeTimeoutSec      = 2
	DefaultServerDBProbeLongTimeoutSec  = 5
	DefaultServerArtifactsTimeoutSec    = 3
	DefaultServerHTTPReadTimeoutSec     = 15
	DefaultServerHTTPWriteTimeoutSec    = 30
	DefaultServerHTTPIdleTimeoutSec     = 60
	DefaultServerListenAddress          = ":8080"
)

// Recent runs read for the ops suggestions.
const DefaultRecentMigrations = 100

// Ops pagination and caps.
const (
	DefaultOpsMigrationsPageDefault = 10
	DefaultOpsMigrationsPageMax     = 100
	DefaultOpsMigrationsPageCap     = 10000
)

// Resources: memory per import worker (MB) and the fraction of the limit workers may use.
const (
	DefaultImportMBPerWorker = 150
	// DefaultMemoryUsageFractionPercent: fraction of container memory used for workers (rest for Go runtime, DB, spikes).
	// Set to 85 to provide more headroom for the Go runtime and database, especially in memory-constrained environments.
	// A lower value may slightly reduce throughput but increases stability by reducing OOM risk.
	DefaultMemoryUsageFractionPercent = 85
)

// Win-probability selection: how hard ml-service may search, and for how many rounds.
const (
	DefaultSelectionMaxWinProbEvalBudget = 500
	// DefaultSelectionBestResponseRounds caps the alternating best-response rounds in
	// win-probability selection. Best response can cycle rather than converge, so the
	// loop is bounded; three rounds is enough for the fixed point when there is one.
	DefaultSelectionBestResponseRounds = 3
)
