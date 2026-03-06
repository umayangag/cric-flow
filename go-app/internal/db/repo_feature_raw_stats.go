package db

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/features"
)

var (
	upsertRawStatsInsertColumns string
	upsertRawStatsValues        string
	upsertRawStatsOnConflict    string
)

func init() {
	names := features.RawStatsFeatureNames()
	const wantRaw = 36
	if len(names) != wantRaw {
		panic("features.RawStatsFeatureNames() must return 18 batting + 18 bowling; got " + strconv.Itoa(len(names)))
	}
	// INSERT columns: fixed prefix + raw stat names + source_version
	upsertRawStatsInsertColumns = "player_id, as_of_date, format_id, scope, scope_id, " + strings.Join(names, ", ") + ", source_version"
	numPlaceholders := 5 + wantRaw + 1
	placeholders := make([]string, numPlaceholders)
	for i := 1; i <= numPlaceholders; i++ {
		placeholders[i-1] = "$" + strconv.Itoa(i)
	}
	upsertRawStatsValues = strings.Join(placeholders, ",")
	// ON CONFLICT DO UPDATE SET: each raw stat column + source_version
	conflictParts := make([]string, 0, wantRaw+1)
	for _, n := range names {
		conflictParts = append(conflictParts, n+"=EXCLUDED."+n)
	}
	conflictParts = append(conflictParts, "source_version=EXCLUDED.source_version")
	upsertRawStatsOnConflict = strings.Join(conflictParts, ", ")
}

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
	query := "INSERT INTO feature_raw_stats_snapshots(" + upsertRawStatsInsertColumns + ") VALUES(" + upsertRawStatsValues + ") ON CONFLICT (player_id, as_of_date, format_id, scope, scope_id) DO UPDATE SET " + upsertRawStatsOnConflict
	_, err := Pool.Exec(
		ctx,
		query,
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
