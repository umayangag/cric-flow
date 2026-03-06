package db

import (
	"context"
	"errors"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/features"
)

// UpsertFeatureRawStatsSnapshot inserts or updates a raw windowed stats snapshot row for the given scope.
// Column order in the INSERT must match features.RawStats field order and the migration
// feature_raw_stats_snapshots table; when adding or reordering raw stat columns, update both the
// migration and this call in lockstep.
func UpsertFeatureRawStatsSnapshot(
	ctx context.Context,
	playerID int64,
	asOf time.Time,
	formatID int64,
	scope string,
	scopeID *int64,
	bat, bowl features.RawStats,
	sourceVersion string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO feature_raw_stats_snapshots(
			player_id, as_of_date, format_id, scope, scope_id,
			batting_mean_w3, batting_mean_w5, batting_mean_w10, batting_mean_w20,
			batting_std_w5, batting_std_w10, batting_max_w10, batting_min_w10, batting_median_w10,
			batting_last_1, batting_last_2, batting_last_3,
			batting_career_mean, batting_career_count, batting_pct_zero_w10, batting_trend_w5,
			batting_days_since_last, batting_innings_in_last_90d,
			bowling_mean_w3, bowling_mean_w5, bowling_mean_w10, bowling_mean_w20,
			bowling_std_w5, bowling_std_w10, bowling_max_w10, bowling_min_w10, bowling_median_w10,
			bowling_last_1, bowling_last_2, bowling_last_3,
			bowling_career_mean, bowling_career_count, bowling_pct_zero_w10, bowling_trend_w5,
			bowling_days_since_last, bowling_innings_in_last_90d,
			source_version
		) VALUES(
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9, $10,$11,$12,$13,$14, $15,$16,$17, $18,$19,$20,$21, $22,$23,
			$24,$25,$26,$27, $28,$29,$30,$31,$32, $33,$34,$35, $36,$37,$38,$39, $40,$41,
			$42
		)
		ON CONFLICT (player_id, as_of_date, format_id, scope, scope_id)
		DO UPDATE SET
			batting_mean_w3=EXCLUDED.batting_mean_w3, batting_mean_w5=EXCLUDED.batting_mean_w5,
			batting_mean_w10=EXCLUDED.batting_mean_w10, batting_mean_w20=EXCLUDED.batting_mean_w20,
			batting_std_w5=EXCLUDED.batting_std_w5, batting_std_w10=EXCLUDED.batting_std_w10,
			batting_max_w10=EXCLUDED.batting_max_w10, batting_min_w10=EXCLUDED.batting_min_w10,
			batting_median_w10=EXCLUDED.batting_median_w10,
			batting_last_1=EXCLUDED.batting_last_1, batting_last_2=EXCLUDED.batting_last_2,
			batting_last_3=EXCLUDED.batting_last_3,
			batting_career_mean=EXCLUDED.batting_career_mean, batting_career_count=EXCLUDED.batting_career_count,
			batting_pct_zero_w10=EXCLUDED.batting_pct_zero_w10, batting_trend_w5=EXCLUDED.batting_trend_w5,
			batting_days_since_last=EXCLUDED.batting_days_since_last,
			batting_innings_in_last_90d=EXCLUDED.batting_innings_in_last_90d,
			bowling_mean_w3=EXCLUDED.bowling_mean_w3, bowling_mean_w5=EXCLUDED.bowling_mean_w5,
			bowling_mean_w10=EXCLUDED.bowling_mean_w10, bowling_mean_w20=EXCLUDED.bowling_mean_w20,
			bowling_std_w5=EXCLUDED.bowling_std_w5, bowling_std_w10=EXCLUDED.bowling_std_w10,
			bowling_max_w10=EXCLUDED.bowling_max_w10, bowling_min_w10=EXCLUDED.bowling_min_w10,
			bowling_median_w10=EXCLUDED.bowling_median_w10,
			bowling_last_1=EXCLUDED.bowling_last_1, bowling_last_2=EXCLUDED.bowling_last_2,
			bowling_last_3=EXCLUDED.bowling_last_3,
			bowling_career_mean=EXCLUDED.bowling_career_mean, bowling_career_count=EXCLUDED.bowling_career_count,
			bowling_pct_zero_w10=EXCLUDED.bowling_pct_zero_w10, bowling_trend_w5=EXCLUDED.bowling_trend_w5,
			bowling_days_since_last=EXCLUDED.bowling_days_since_last,
			bowling_innings_in_last_90d=EXCLUDED.bowling_innings_in_last_90d,
			source_version=EXCLUDED.source_version`,
		playerID, asOf, formatID, scope, scopeID,
		bat.MeanW3, bat.MeanW5, bat.MeanW10, bat.MeanW20,
		bat.StdW5, bat.StdW10, bat.MaxW10, bat.MinW10, bat.MedianW10,
		bat.Last1, bat.Last2, bat.Last3,
		bat.CareerMean, bat.CareerCount, bat.PctZeroW10, bat.TrendW5,
		bat.DaysSinceLast, bat.InningsInLast90D,
		bowl.MeanW3, bowl.MeanW5, bowl.MeanW10, bowl.MeanW20,
		bowl.StdW5, bowl.StdW10, bowl.MaxW10, bowl.MinW10, bowl.MedianW10,
		bowl.Last1, bowl.Last2, bowl.Last3,
		bowl.CareerMean, bowl.CareerCount, bowl.PctZeroW10, bowl.TrendW5,
		bowl.DaysSinceLast, bowl.InningsInLast90D,
		sourceVersion,
	)
	return err
}
