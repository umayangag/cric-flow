package db

import (
	"context"
	"errors"
	"time"
)

// FeatureProvider abstracts retrieval of player features strictly as-of a cutoff date.
// Only precomputed feature stores must be used (no averages or on-the-fly values for ML).
type FeatureProvider interface {
	// GetPlayerFeaturesAtCutoff returns a map of playerID -> featureName->value for
	// the given players, using only data available at-or-before the cutoff timestamp.
	GetPlayerFeaturesAtCutoff(
		ctx context.Context,
		cutoff time.Time,
		playerIDs []int64,
	) (map[int64]map[string]float64, error)
}

// DefaultFeatureProvider no longer returns average-based features. Use exportqueries.ComputeFeaturesAtCutoffForMatch
// (when matchID > 0) or exportqueries.ComputeFeaturesAtCutoffNoMatch (when no match, pass format) for precomputed-only features.
type DefaultFeatureProvider struct{}

// DefaultFeatureProviderInst is the default instance (e.g. for tests that need a no-op or error).
var DefaultFeatureProviderInst FeatureProvider = &DefaultFeatureProvider{}

// GetPlayerFeaturesAtCutoff implements FeatureProvider. It does not use averages; returns an error so callers
// use the precomputed path (ComputeFeaturesAtCutoffForMatch or ComputeFeaturesAtCutoffNoMatch with format).
func (p *DefaultFeatureProvider) GetPlayerFeaturesAtCutoff(
	ctx context.Context,
	cutoff time.Time,
	playerIDs []int64,
) (map[int64]map[string]float64, error) {
	if cutoff.IsZero() {
		return nil, errors.New("cutoff time required")
	}
	if len(playerIDs) == 0 {
		return map[int64]map[string]float64{}, nil
	}
	return nil, errors.New("player features require precomputed values only (no averages); use exportqueries.ComputeFeaturesAtCutoffForMatch with matchID or ComputeFeaturesAtCutoffNoMatch with format when no match")
}
