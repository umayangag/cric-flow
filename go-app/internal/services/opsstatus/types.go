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
	Timestamp string          `json:"timestamp"`
	Services  map[string]bool `json:"services"`
	DB        map[string]any  `json:"db"`
	Dataset   map[string]any  `json:"dataset"`
	Artifacts map[string]any  `json:"artifacts"`
	Fielding  map[string]any  `json:"fielding"`
	// Freshness is the one freshness verdict every surface reads (P2-1). It replaced
	// `db_freshness`, whose 7/30 buckets were a second rule with a second threshold.
	Freshness      Freshness      `json:"freshness"`
	DBCompleteness map[string]any `json:"db_completeness"`
	Pipeline       map[string]any `json:"pipeline,omitempty"`
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

// worseStatus computes the worse (higher severity) status between a and b, for the
// completeness section's roll-up. Severity order: ok < missing < unknown.
func worseStatus(a, b string) string {
	rank := map[string]int{"ok": 0, "missing": 1, "unknown": 2}
	if rank[a] >= rank[b] {
		return a
	}
	return b
}
