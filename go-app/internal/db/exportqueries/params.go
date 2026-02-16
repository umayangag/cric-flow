package exportqueries

import (
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// GetFeatureExtractionParams returns EWM alpha, consistency last-N, and form window N
// from config when set and valid; otherwise the package defaults.
// Used by training-data export (batting, bowling, fielding) so feature extraction
// is config-driven and consistent with precompute when it uses config defaults.
func GetFeatureExtractionParams() (alpha float64, lastN, windowN int) {
	alpha = DefaultEWMAlpha
	lastN = DefaultConsistencyLastN
	windowN = DefaultFormWindowN
	cfg := config.Load()
	if cfg == nil {
		return alpha, lastN, windowN
	}
	if cfg.Features.EWMAlpha > 0 && cfg.Features.EWMAlpha <= 1 {
		alpha = cfg.Features.EWMAlpha
	}
	if cfg.Features.ConsistencyLastN > 0 {
		lastN = cfg.Features.ConsistencyLastN
	}
	if cfg.Features.FormWindowN >= 0 {
		windowN = cfg.Features.FormWindowN
	}
	return alpha, lastN, windowN
}
