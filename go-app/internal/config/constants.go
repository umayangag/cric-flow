package config

import "time"

// DefaultTimeout is the default timeout for long-running CLI operations (imports, precompute, etc.).
// Set to 1 year so runs effectively have no deadline; use -timeout to cap (e.g. -timeout=5h).
const DefaultTimeout = 7 * 24 * time.Hour

// Feature extraction defaults used when config is missing or values are zero.
// These affect prediction results; keep in sync with config.json defaults.
const (
	DefaultFeatureEWMAlpha         = 0.3
	DefaultFeatureConsistencyLastN = 10
	DefaultFeatureFormWindowN      = 0
)

// Fielding enrich defaults (used when ML has no fielding model; go-app enriches from history).
const (
	DefaultFieldingEWMAlpha         = 0.3
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
