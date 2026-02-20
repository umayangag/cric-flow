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

// Score weights for combining batting/bowling/fielding signals in team selection.
// Used when selection.score_weights is not configured.
const (
	DefaultScoreWeightBat         = 0.45
	DefaultScoreWeightBowl        = 0.40
	DefaultScoreWeightField       = 0.10
	DefaultScoreWeightKeeperBonus = 0.02
)

// Default score normalization divisors when selection.score_normalization is not set.
// Format-agnostic fallbacks (T20/ODI typical).
const (
	DefaultScoreNormBatDivisor    = 80
	DefaultScoreNormWicketDivisor = 5
	DefaultScoreNormEconBase      = 12
	DefaultScoreNormFieldDivisor  = 5
)

// Backtest / export-contributions defaults.
const (
	DefaultExportMaxMatchIDs = 200
)

// Monte Carlo simulation defaults (predictor.max_total_samples, top_k, samples_per_matchup, and simulation CVs).
const (
	DefaultMaxTotalSamples                = 100000
	DefaultSimulationTopKPerTeam          = 50
	DefaultSimulationNumSamplesPerMatchup = 500
	DefaultSimulationRunsCV               = 0.35
	DefaultSimulationWicketsCV            = 0.4
	DefaultSimulationEconomyCV            = 0.15
)
