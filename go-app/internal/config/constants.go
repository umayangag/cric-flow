package config

import (
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

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

// DefaultPoolRecencyMonths is the measured default recency window per format, in months
// (D-12). Over the last year of real matches, it is the smallest window covering >= 95%
// of the players who actually took the field, of those the all-time pool would have
// offered at all: TEST 97.5%, ODI 95.3%, T20 96.7% at twelve months; T20I 95.1% at nine.
// Those windows cut the all-time pool to 45%, 43%, 52% and 25% of its size respectively.
// The full table is in docs/EXTERNAL_DATA_PLAN.md § D-12.
var DefaultPoolRecencyMonths = map[string]int{
	formats.CodeTest: 12,
	formats.CodeODI:  12,
	formats.CodeT20:  12,
	formats.CodeT20I: 9,
}

// Retirement ledger defaults (D-12).
const (
	// DefaultPoolRecencyMonthsFallback is the window for a format neither the config
	// nor DefaultPoolRecencyMonths names. It is a bound, not all-time: an unbounded
	// pool is the defect D-12 fixes, so an unknown format gets the widest measured
	// window rather than none.
	DefaultPoolRecencyMonthsFallback = 12

	// DefaultRetirementInactiveYears is criterion (a)'s bound. Measured: over the last
	// year of real matches, 0.060% of the player-matches actually fielded were a
	// return after a five-year absence from every format, against 0.117% at four years
	// and 0.228% at three.
	DefaultRetirementInactiveYears = 5

	// DefaultRetirementAgeBoundYears is criterion (c)'s age bound for a format the
	// config does not name. Provisional: the criterion reports itself unavailable
	// until X-1a supplies dates of birth, so this number has no effect yet.
	DefaultRetirementAgeBoundYears = 40

	// DefaultRetirementAgeInactiveYears is criterion (c)'s inactivity half. Two years
	// is where a return becomes uncommon (0.554% of fielded player-matches); it
	// corroborates only in combination with the age bound, never on its own.
	DefaultRetirementAgeInactiveYears = 2
)

// Win-probability selection: how hard ml-service may search, and for how many rounds.
const (
	DefaultSelectionMaxWinProbEvalBudget = 500
	// DefaultSelectionBestResponseRounds caps the alternating best-response rounds in
	// win-probability selection. Best response can cycle rather than converge, so the
	// loop is bounded; three rounds is enough for the fixed point when there is one.
	DefaultSelectionBestResponseRounds = 3
)
