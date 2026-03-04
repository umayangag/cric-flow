package db

import (
	"context"
	"errors"
	"time"
)

// UpsertFeatureFormSnapshot inserts or updates a consolidated form snapshot row for the given scope.
func UpsertFeatureFormSnapshot(
	ctx context.Context,
	playerID int64,
	asOf time.Time,
	formatID int64,
	scope string, // 'overall' | 'venue' | 'opposition'
	scopeID *int64, // nil when scope == 'overall'
	battingValue float64,
	bowlingValue float64,
	alpha float64,
	nSamplesBat float64,
	nSamplesBowl float64,
	effectiveN float64,
	sourceVersion string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO feature_form_snapshots(
			player_id, as_of_date, format_id, scope, scope_id,
			batting_value, bowling_value, alpha, n_samples_bat, n_samples_bowl, effective_n, source_version
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (player_id, as_of_date, format_id, scope, scope_id)
		DO UPDATE SET batting_value = EXCLUDED.batting_value,
			bowling_value = EXCLUDED.bowling_value,
			alpha = EXCLUDED.alpha,
			n_samples_bat = EXCLUDED.n_samples_bat,
			n_samples_bowl = EXCLUDED.n_samples_bowl,
			effective_n = EXCLUDED.effective_n,
			source_version = EXCLUDED.source_version`,
		playerID,
		asOf,
		formatID,
		scope,
		scopeID,
		battingValue,
		bowlingValue,
		alpha,
		nSamplesBat,
		nSamplesBowl,
		effectiveN,
		sourceVersion,
	)
	return err
}

// UpsertFeatureConsistencySnapshot inserts or updates a consolidated consistency snapshot row for the given scope.
func UpsertFeatureConsistencySnapshot(
	ctx context.Context,
	playerID int64,
	asOf time.Time,
	formatID int64,
	scope string, // 'overall' | 'venue' | 'opposition'
	scopeID *int64, // nil when scope == 'overall'
	battingValue float64,
	bowlingValue float64,
	windowN int,
	nSamplesBat int,
	nSamplesBowl int,
	sourceVersion string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO feature_consistency_snapshots(
			player_id, as_of_date, format_id, scope, scope_id,
			batting_value, bowling_value, window_n, n_samples_bat, n_samples_bowl, source_version
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (player_id, as_of_date, format_id, scope, scope_id)
		DO UPDATE SET batting_value = EXCLUDED.batting_value,
			bowling_value = EXCLUDED.bowling_value,
			window_n = EXCLUDED.window_n,
			n_samples_bat = EXCLUDED.n_samples_bat,
			n_samples_bowl = EXCLUDED.n_samples_bowl,
			source_version = EXCLUDED.source_version`,
		playerID,
		asOf,
		formatID,
		scope,
		scopeID,
		battingValue,
		bowlingValue,
		windowN,
		nSamplesBat,
		nSamplesBowl,
		sourceVersion,
	)
	return err
}
