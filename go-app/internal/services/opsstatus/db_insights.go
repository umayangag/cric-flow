package opsstatus

import (
	"context"
	"log/slog"
	"time"
)

// BuildDBCompletenessSection counts matches over the last 30 days and classifies status.
//
// It asks a different question from the freshness object beside it — "is the import
// empty?", not "how old is what we serve?" — which is why P2-1 left it alone when it
// deleted `db_freshness`'s buckets. Its `ok | missing | unknown` says whether a format
// has any recent rows at all; nothing here decides whether a prediction is served.
func BuildDBCompletenessSection(ctx context.Context, probe InsightsProbe, now time.Time) map[string]any {
	formats := CricketFormatCodes
	section := map[string]any{
		"formats": map[string]any{},
		"overall": map[string]any{"status": "unknown"},
	}
	fm := map[string]any{}
	worst := "ok"
	since := now.AddDate(0, 0, -30)
	var totalLast30d int64

	for _, f := range formats {
		st := map[string]any{"status": "unknown", "expected_min_30d": 1}
		if probe == nil {
			fm[f] = st
			worst = worseStatus(worst, "unknown")
			continue
		}
		n, err := probe.CountMatchesSinceByFormat(ctx, f, since)
		if err != nil {
			slog.Error("failed to count matches since", "format", f, "since", since, "err", err)
			st["status"] = "unknown"
			fm[f] = st
			worst = worseStatus(worst, "unknown")
			continue
		}
		st["matches_last_30d"] = n
		totalLast30d += n
		status := "missing"
		if n >= 1 {
			status = "ok"
		}
		st["status"] = status
		fm[f] = st
		worst = worseStatus(worst, status)
	}
	section["formats"] = fm
	section["overall"] = map[string]any{"status": worst, "matches_last_30d": totalLast30d}
	return section
}
