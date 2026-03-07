package db

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/features"
)

// safeSQLIdentifier matches names that are safe to use as column names (no SQL injection).
var safeSQLIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

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
	for i, n := range names {
		if !safeSQLIdentifier.MatchString(n) {
			panic(
				"features.RawStatsFeatureNames()[" + strconv.Itoa(
					i,
				) + "] invalid column name (SQL injection risk): " + n,
			)
		}
	}
	// INSERT columns: fixed prefix + raw stat names + source_version
	upsertRawStatsInsertColumns = "player_id, as_of_date, format_id, scope, scope_id, " + strings.Join(
		names,
		", ",
	) + ", source_version"
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
	args := make([]any, 0, 5+18+18+1)
	args = append(args, playerID, asOf, formatID, scope, scopeID)
	args = append(args, bat.Values()...)
	args = append(args, bowl.Values()...)
	args = append(args, sourceVersion)
	_, err := Pool.Exec(ctx, query, args...)
	return err
}
