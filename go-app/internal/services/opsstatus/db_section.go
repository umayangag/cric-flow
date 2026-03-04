package opsstatus

import (
	"context"
	"time"
)

// BuildDBSection assembles the DB section map for /ops/status using the provided probe.
func BuildDBSection(ctx context.Context, probe DBProbe) map[string]any {
	out := map[string]any{"connected": false}
	if probe == nil {
		out["migration"] = map[string]any{"status": "unknown"}
		return out
	}
	if err := probe.Ping(ctx); err == nil {
		out["connected"] = true
		counts := map[string]int64{}
		if n, err := probe.Count(ctx, "players"); err == nil {
			counts["players"] = n
		}
		if n, err := probe.Count(ctx, "matches"); err == nil {
			counts["matches"] = n
		}
		if len(counts) > 0 {
			out["counts"] = counts
		}
		if cur, exp, status, err := probe.MigrationInfo(ctx); err == nil {
			out["migration"] = map[string]any{"status": status, "current": cur, "expected": exp}
		} else {
			out["migration"] = map[string]any{"status": "unknown"}
		}
		if last, err := probe.LastMatchImportAt(ctx); err == nil && !last.IsZero() {
			out["last_match_import_at"] = last.UTC().Format(time.RFC3339)
		}
		if stats, err := probe.TableStats(ctx); err == nil {
			out["table_stats"] = stats
		}
		return out
	}
	// Disconnected path
	if _, exp, status, _ := probe.MigrationInfo(ctx); exp > 0 {
		out["migration"] = map[string]any{"status": status, "expected": exp}
	} else {
		out["migration"] = map[string]any{"status": "unknown"}
	}
	return out
}
