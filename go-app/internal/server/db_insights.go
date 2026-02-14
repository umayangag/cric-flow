package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// insightsProbe defines minimal methods required to compute DB freshness and completeness.
type insightsProbe interface {
	LatestMatchDateByFormat(ctx context.Context, format string) (time.Time, error)
	CountMatchesSinceByFormat(ctx context.Context, format string, since time.Time) (int64, error)
}

// productionInsightsProbe implements insightsProbe using the real database connection.
type productionInsightsProbe struct{}

func (productionInsightsProbe) LatestMatchDateByFormat(ctx context.Context, format string) (time.Time, error) {
	if db.Pool == nil {
		return time.Time{}, errDBNotInitialized
	}
	// Prefer match_date for latest activity.
	// We don't use updated_at here anymore to avoid complexity if column is missing,
	// and match_date is more accurate for "freshness" of the data content itself.
	var ts time.Time
	if err := db.Pool.QueryRow(ctx, `
        SELECT COALESCE(MAX(m.match_date), DATE '0001-01-01')
        FROM match m
        JOIN match_format mf ON m.format_id = mf.id
        WHERE mf.code = $1
    `, format).Scan(&ts); err != nil {
		return time.Time{}, err
	}
	if ts.IsZero() {
		return time.Time{}, nil
	}
	return ts.UTC(), nil
}

func (productionInsightsProbe) CountMatchesSinceByFormat(
	ctx context.Context,
	format string,
	since time.Time,
) (int64, error) {
	if db.Pool == nil {
		return 0, errDBNotInitialized
	}
	// Count distinct matches since the provided boundary using match_date.
	var n int64
	if err := db.Pool.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM match m
        JOIN match_format mf ON m.format_id = mf.id
        WHERE mf.code = $1 AND m.match_date >= $2::date
    `, format, since).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Internal error value to signal db not initialized; kept local to avoid importing std errors repeatedly.
var errDBNotInitialized = fmtError("db pool not initialized")

type fmtError string

func (e fmtError) Error() string { return string(e) }

// buildDBFreshnessSection computes latest per-format match date, days since, and status.
func buildDBFreshnessSection(ctx context.Context, probe insightsProbe, now time.Time) map[string]any {
	formats := getCricketFormats()
	section := map[string]any{
		"formats": map[string]any{},
		"overall": map[string]any{"status": "unknown"},
	}
	fm := map[string]any{}
	worst := "ok"

	for _, f := range formats {
		st := map[string]any{"status": "missing"}
		if probe == nil {
			st["status"] = "unknown"
			fm[f] = st
			worst = worseStatus(worst, "unknown")
			continue
		}
		t, err := probe.LatestMatchDateByFormat(ctx, f)
		if err != nil {
			slog.Error("failed to get latest match date", "format", f, "err", err)
			st["status"] = "unknown"
			fm[f] = st
			worst = worseStatus(worst, "unknown")
			continue
		}
		if t.IsZero() {
			st["status"] = "missing"
			fm[f] = st
			worst = worseStatus(worst, "missing")
			continue
		}
		// compute days since
		days := int(now.Sub(t.UTC()).Hours() / 24)
		st["latest_match_date"] = t.UTC().Format("2006-01-02")
		st["days_since"] = days
		var status string
		switch {
		case days <= 7:
			status = "ok"
		case days <= 30:
			status = "stale"
		default:
			status = "missing"
		}
		st["status"] = status
		fm[f] = st
		worst = worseStatus(worst, status)
	}
	section["formats"] = fm
	section["overall"] = map[string]any{"status": worst}
	return section
}

// buildDBCompletenessSection counts matches over the last 30 days and classifies status.
func buildDBCompletenessSection(ctx context.Context, probe insightsProbe, now time.Time) map[string]any {
	formats := getCricketFormats()
	section := map[string]any{
		"formats": map[string]any{},
		"overall": map[string]any{"status": "unknown"},
	}
	fm := map[string]any{}
	worst := "ok"
	since := now.AddDate(0, 0, -30)

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
		status := "missing"
		if n >= 1 {
			status = "ok"
		}
		st["status"] = status
		fm[f] = st
		worst = worseStatus(worst, status)
	}
	section["formats"] = fm
	section["overall"] = map[string]any{"status": worst}
	return section
}

// worseStatus computes the worse (higher severity) status between a and b.
// Severity order: ok < stale < missing < unknown
func worseStatus(a, b string) string {
	rank := map[string]int{"ok": 0, "stale": 1, "missing": 2, "unknown": 3}
	if rank[a] >= rank[b] {
		return a
	}
	return b
}
