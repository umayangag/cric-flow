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
	// Only require a DB pool when using the real poolQuerier. Unit tests replace
	// featureQuerier with a fake that doesn't need Pool.
	if _, usesPool := featureQuerier.(poolQuerier); usesPool && Pool == nil {
		return nil, errors.New("db pool not initialized")
	}

	// Initialize the output map with all player IDs.
	out := make(map[int64]map[string]float64, len(playerIDs))
	for _, pid := range playerIDs {
		out[pid] = make(map[string]float64)
	}

	// Fast path: when using the real pool, batch the queries to avoid N+1.
	// Note: player.batting_consistency/bowling_consistency and player_form_data were removed
	// in migrations (0007, 0008, 0015). We use only batting_data/bowling_data aggregates here;
	// consistency defaults are applied by the ML service when missing.
	if _, usesPool := featureQuerier.(poolQuerier); usesPool {
		// 1) Batting aggregates up to cutoff
		if rows, err := Pool.Query(ctx, `
            SELECT bd.player_id, COALESCE(AVG(bd.runs), 0)
            FROM batting_data bd
            JOIN match m ON m.match_id = bd.match_id
            WHERE bd.player_id = ANY($1::bigint[]) AND m.match_date <= $2
            GROUP BY bd.player_id
        `, playerIDs, cutoff); err == nil {
			defer rows.Close()
			for rows.Next() {
				var pid int64
				var avgRuns sql.NullFloat64
				if err := rows.Scan(&pid, &avgRuns); err == nil {
					feats := out[pid]
					if feats == nil {
						feats = make(map[string]float64)
						out[pid] = feats
					}
					if avgRuns.Valid {
						if _, ok := feats["batting_form"]; !ok {
							feats["batting_form"] = avgRuns.Float64
						}
						feats["avg_runs"] = avgRuns.Float64
					}
				}
			}
		}

		// 2) Bowling aggregates up to cutoff
		if rows, err := Pool.Query(ctx, `
            SELECT bw.player_id, COALESCE(AVG(bw.wickets), 0), COALESCE(AVG(bw.econ), 0)
            FROM bowling_data bw
            JOIN match m ON m.match_id = bw.match_id
            WHERE bw.player_id = ANY($1::bigint[]) AND m.match_date <= $2
            GROUP BY bw.player_id
        `, playerIDs, cutoff); err == nil {
			defer rows.Close()
			for rows.Next() {
				var pid int64
				var avgWkts, avgEcon sql.NullFloat64
				if err := rows.Scan(&pid, &avgWkts, &avgEcon); err == nil {
					feats := out[pid]
					if feats == nil {
						feats = make(map[string]float64)
						out[pid] = feats
					}
					if avgWkts.Valid {
						if _, ok := feats["bowling_form"]; !ok {
							feats["bowling_form"] = avgWkts.Float64
						}
						feats["avg_wickets"] = avgWkts.Float64
					}
					if avgEcon.Valid {
						feats["avg_economy"] = avgEcon.Float64
					}
				}
			}
		}

		return out, nil
	}

	// Fallback path (tests or custom querier): per-player aggregate queries only.
	// player.batting_consistency/bowling_consistency and player_form_data no longer exist.
	for _, pid := range playerIDs {
		feats := out[pid]
		if feats == nil {
			feats = make(map[string]float64)
		}
		var avgRuns sql.NullFloat64
		_ = featureQuerier.QueryRow(ctx, `
            SELECT COALESCE(AVG(bd.runs), 0)
            FROM batting_data bd
            JOIN match m ON m.match_id = bd.match_id
            WHERE bd.player_id = $1 AND m.match_date <= $2
        `, pid, cutoff).Scan(&avgRuns)
		if avgRuns.Valid {
			if _, ok := feats["batting_form"]; !ok {
				feats["batting_form"] = avgRuns.Float64
			}
			feats["avg_runs"] = avgRuns.Float64
		}

		var avgWkts, avgEcon sql.NullFloat64
		_ = featureQuerier.QueryRow(ctx, `
            SELECT COALESCE(AVG(bw.wickets), 0), COALESCE(AVG(bw.econ), 0)
            FROM bowling_data bw
            JOIN match m ON m.match_id = bw.match_id
            WHERE bw.player_id = $1 AND m.match_date <= $2
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
// - Uses md.match_date <= cutoff for base table fallbacks to preserve strict cutoff semantics.
// - Returns partial maps per player without failing the whole call on missing rows.
