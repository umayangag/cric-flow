// Package opsstatus assembles the /ops/status response from DB probes,
// filesystem scans, ML service health checks, and tracking state.
package opsstatus

import (
	"context"
	"time"

	formatsPkg "github.com/umayangag/cric-flow/go-app/internal/formats"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// Response is the top-level JSON returned by /ops/status.
type Response struct {
	Timestamp      string          `json:"timestamp"`
	Services       map[string]bool `json:"services"`
	DB             map[string]any  `json:"db"`
	Precompute     map[string]any  `json:"precompute"`
	Dataset        map[string]any  `json:"dataset"`
	Exports        map[string]any  `json:"exports"`
	Artifacts      map[string]any  `json:"artifacts"`
	Fielding       map[string]any  `json:"fielding"`
	Weather        map[string]any  `json:"weather"`
	DBFreshness    map[string]any  `json:"db_freshness"`
	DBCompleteness map[string]any  `json:"db_completeness"`
	Pipeline       map[string]any  `json:"pipeline,omitempty"`
}

// DBProbe defines the minimal DB checks needed for /ops/status.
type DBProbe interface {
	Ping(ctx context.Context) error
	Count(ctx context.Context, table string) (int64, error)
	CountFieldingByFormat(ctx context.Context, format string) (int64, error)
	CountFieldingByFormatGrouped(ctx context.Context) (map[string]int64, error)
	MigrationInfo(ctx context.Context) (int, int, string, error)
	LastMatchImportAt(ctx context.Context) (time.Time, error)
	TableStats(ctx context.Context) ([]db.TableStat, error)
}

// InsightsProbe defines minimal methods required to compute DB freshness and completeness.
type InsightsProbe interface {
	LatestMatchDateByFormat(ctx context.Context, format string) (time.Time, error)
	CountMatchesByFormat(ctx context.Context, format string) (int64, error)
	CountMatchesSinceByFormat(ctx context.Context, format string, since time.Time) (int64, error)
}

// CricketFormatCodes are the shared format codes used across ops status helpers.
// Sourced from internal/formats so there is one canonical list.
var CricketFormatCodes = formatsPkg.CanonicalCodes()

// worseStatus computes the worse (higher severity) status between a and b.
// Severity order: ok < stale < missing < unknown
func worseStatus(a, b string) string {
	rank := map[string]int{"ok": 0, "stale": 1, "missing": 2, "unknown": 3}
	if rank[a] >= rank[b] {
		return a
	}
	return b
}
