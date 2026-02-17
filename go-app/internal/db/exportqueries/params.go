package exportqueries

import (
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// GetFeatureExtractionParams returns EWM alpha, consistency last-N, form window N,
// alphaShort, alphaLong for multi-horizon form, and momentumLastN.
// Used by training-data export (batting, bowling, fielding) so feature extraction
// is config-driven and consistent with precompute when it uses config defaults.
func GetFeatureExtractionParams() (alpha float64, lastN, windowN int, alphaShort, alphaLong float64, momentumLastN int) {
	alpha = config.DefaultFeatureEWMAlpha
	lastN = config.DefaultFeatureConsistencyLastN
	windowN = config.DefaultFeatureFormWindowN
	alphaShort = config.DefaultFeatureEWMAlphaShort
	alphaLong = config.DefaultFeatureEWMAlphaLong
	momentumLastN = config.DefaultFeatureMomentumLastN
	cfg := config.Load()
	if cfg == nil {
		return alpha, lastN, windowN, alphaShort, alphaLong, momentumLastN
	}
	if cfg.Features.EWMAlpha > 0 && cfg.Features.EWMAlpha <= 1 {
		alpha = cfg.Features.EWMAlpha
	}
	if cfg.Features.EWMAlphaShort > 0 && cfg.Features.EWMAlphaShort <= 1 {
		alphaShort = cfg.Features.EWMAlphaShort
	}
	if cfg.Features.EWMAlphaLong > 0 && cfg.Features.EWMAlphaLong <= 1 {
		alphaLong = cfg.Features.EWMAlphaLong
	}
	if cfg.Features.ConsistencyLastN > 0 {
		lastN = cfg.Features.ConsistencyLastN
	}
	if cfg.Features.FormWindowN >= 0 {
		windowN = cfg.Features.FormWindowN
	}
	if cfg.Features.MomentumLastN > 0 {
		momentumLastN = cfg.Features.MomentumLastN
	}
	return alpha, lastN, windowN, alphaShort, alphaLong, momentumLastN
}
