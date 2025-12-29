package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// FeatureProvider abstracts retrieval of player features strictly as-of a cutoff date.
// Implementations should prefer precomputed feature stores, and fall back to on-the-fly
// computation when precomputed rows are unavailable.
type FeatureProvider interface {
	// GetPlayerFeaturesAtCutoff returns a map of playerID -> featureName->value for
	// the given players, using only data available at-or-before the cutoff timestamp.
	GetPlayerFeaturesAtCutoff(
		ctx context.Context,
		cutoff time.Time,
		playerIDs []int64,
	) (map[int64]map[string]float64, error)
}

// --- minimal querier seam to allow unit testing without a live DB ---

// rowScanner abstracts Scan on returned rows.
type rowScanner interface{ Scan(dest ...any) error }

// simpleQuerier abstracts the subset of DB calls we need.
type simpleQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) rowScanner
}

// poolQuerier bridges the global Pool to the simpleQuerier interface.
type poolQuerier struct{}

func (poolQuerier) QueryRow(ctx context.Context, q string, args ...any) rowScanner {
	return Pool.QueryRow(ctx, q, args...)
}

// featureQuerier provides an injectable seam for tests. Defaults to poolQuerier.
var featureQuerier simpleQuerier = poolQuerier{}

// DefaultFeatureProvider is a placeholder provider. It returns an empty feature map
// while establishing the contract and cutoff enforcement surface. This can be replaced
// with a real implementation that reads precomputed tables or computes features ad hoc.
type DefaultFeatureProvider struct{}

// DefaultFeatureProviderInst is the default instance used by server seams.
var DefaultFeatureProviderInst FeatureProvider = &DefaultFeatureProvider{}

// GetPlayerFeaturesAtCutoff implements FeatureProvider. For now it validates input and
// returns an empty map to keep current flows non-blocking.
func (p *DefaultFeatureProvider) GetPlayerFeaturesAtCutoff(
	ctx context.Context,
	cutoff time.Time,
	playerIDs []int64,
) (map[int64]map[string]float64, error) {
	// Guard: cutoff must be set (non-zero) and at least one player id provided.
	if cutoff.IsZero() {
		return nil, errors.New("cutoff time required")
	}
	if len(playerIDs) == 0 {
		return map[int64]map[string]float64{}, nil
	}
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}

	out := make(map[int64]map[string]float64, len(playerIDs))
	// 1) Precomputed-first: try to read form/consistency-like values.
	for _, pid := range playerIDs {
		feats := make(map[string]float64)
		// Player-level consistency from player table (if available)
		var batCons, bowlCons sql.NullFloat64
		_ = featureQuerier.QueryRow(ctx, `
            SELECT batting_consistency, bowling_consistency
            FROM player WHERE id = $1
        `, pid).Scan(&batCons, &bowlCons)
		if batCons.Valid {
			feats["batting_consistency"] = batCons.Float64
		}
		if bowlCons.Valid {
			feats["bowling_consistency"] = bowlCons.Float64
		}

		// Player form from player_form_data using season <= cutoff's season (best-effort)
		// We treat season_name as text; filter lexicographically (works for YYYY)
		cutoffYear := cutoff.Year()
		var batForm, bowlForm sql.NullFloat64
		_ = featureQuerier.QueryRow(ctx, `
            SELECT pfd.batting_form, pfd.bowling_form
            FROM player_form_data pfd
            JOIN season s ON s.id = pfd.season_id
            WHERE pfd.player_id = $1 AND s.season_name <= $2
            ORDER BY s.season_name DESC
            LIMIT 1
        `, pid, cutoffYear).Scan(&batForm, &bowlForm)
		if batForm.Valid {
			feats["batting_form"] = batForm.Float64
		}
		if bowlForm.Valid {
			feats["bowling_form"] = bowlForm.Float64
		}

		out[pid] = feats
	}

	// 2) Fallback computation for missing basic signals using base tables strictly before cutoff.
	for _, pid := range playerIDs {
		feats := out[pid]
		if feats == nil {
			feats = make(map[string]float64)
		}

		// Batting fallback: average runs up to cutoff
		var avgRuns sql.NullFloat64
		_ = featureQuerier.QueryRow(ctx, `
            SELECT COALESCE(AVG(bd.runs), 0)
            FROM batting_data bd
            JOIN match_details md ON md.match_id = bd.match_id
            WHERE bd.player_id = $1 AND md.date <= $2
        `, pid, cutoff).Scan(&avgRuns)
		if avgRuns.Valid {
			if _, ok := feats["batting_form"]; !ok {
				feats["batting_form"] = avgRuns.Float64
			}
			feats["avg_runs"] = avgRuns.Float64
		}

		// Bowling fallback: average wickets and economy up to cutoff
		var avgWkts, avgEcon sql.NullFloat64
		_ = featureQuerier.QueryRow(ctx, `
            SELECT COALESCE(AVG(bw.wickets), 0), COALESCE(AVG(bw.econ), 0)
            FROM bowling_data bw
            JOIN match_details md ON md.match_id = bw.match_id
            WHERE bw.player_id = $1 AND md.date <= $2
        `, pid, cutoff).Scan(&avgWkts, &avgEcon)
		if avgWkts.Valid {
			if _, ok := feats["bowling_form"]; !ok {
				feats["bowling_form"] = avgWkts.Float64
			}
			feats["avg_wickets"] = avgWkts.Float64
		}
		if avgEcon.Valid {
			feats["avg_economy"] = avgEcon.Float64
		}
		out[pid] = feats
	}

	return out, nil
}

// NOTE: This initial implementation is intentionally conservative and best-effort:
// - Uses season <= cutoff year for precomputed form when explicit timestamps are absent.
// - Uses md.date <= cutoff for base table fallbacks to preserve strict cutoff semantics.
// - Returns partial maps per player without failing the whole call on missing rows.
