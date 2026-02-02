package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	// Prefer updated_at if present, else fall back to date
	var ts time.Time
	// Try updated_at; if column missing, query error will occur and we fall back to date
	if err := db.Pool.QueryRow(ctx, `
        SELECT COALESCE(MAX(updated_at), TO_TIMESTAMP(0))
        FROM match_details
        WHERE format = $1
    `, format).Scan(&ts); err == nil && !ts.IsZero() {
		return ts.UTC(), nil
	}
	if err := db.Pool.QueryRow(ctx, `
        SELECT COALESCE(MAX(date), DATE '0001-01-01')
        FROM match_details
        WHERE format = $1
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
	// Count distinct matches since the provided boundary, using updated_at if exists; else date
	var n int64
	// Try updated_at first
	if err := db.Pool.QueryRow(ctx, `
        SELECT COUNT(DISTINCT match_id)
        FROM match_details
        WHERE format = $1 AND (
            (updated_at IS NOT NULL AND updated_at >= $2)
            OR (updated_at IS NULL AND date >= $3::date)
        )
    `, format, since, since).Scan(&n); err == nil {
		return n, nil
	}
	if err := db.Pool.QueryRow(ctx, `
        SELECT COUNT(DISTINCT match_id)
        FROM match_details
        WHERE format = $1 AND date >= $2::date
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

// buildDBInsightsSuggestions produces suggestions from db_freshness and db_completeness sections.
func buildDBInsightsSuggestions(dbFreshness map[string]any, dbCompleteness map[string]any) []map[string]any {
	var out []map[string]any
	formats := []string{"TEST", "ODI", "T20I", "T20"}

	// Freshness-based suggestions
	if dbFreshness != nil {
		if fmAny, ok := dbFreshness["formats"].(map[string]any); ok {
			var stale, missing []string
			var details []string
			for _, f := range formats {
				if v, ok := fmAny[f].(map[string]any); ok {
					st, _ := v["status"].(string)
					switch st {
					case "stale":
						stale = append(stale, f)
						if d, ok := v["days_since"].(int); ok {
							details = append(details, fmt.Sprintf("%s:%dd", f, d))
						} else if d64, ok := v["days_since"].(float64); ok {
							details = append(details, fmt.Sprintf("%s:%dd", f, int(d64)))
						}
					case "missing":
						missing = append(missing, f)
					}
				}
			}
			if len(stale) > 0 {
				sort.Strings(stale)
				msg := fmt.Sprintf("Data is stale for %s", strings.Join(stale, ","))
				if len(details) > 0 {
					sort.Strings(details)
					msg += fmt.Sprintf(" (days since: %s)", strings.Join(details, ","))
				}
				out = append(out, map[string]any{
					"reason":   msg,
					"commands": []string{"make cricsheet-import", "make precompute"},
				})
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				out = append(out, map[string]any{
					"reason":   fmt.Sprintf("Data missing for %s (no recent matches)", strings.Join(missing, ",")),
					"commands": []string{"make cricsheet-import"},
				})
			}
		}
	}

	// Completeness-based suggestions
	if dbCompleteness != nil {
		if fmAny, ok := dbCompleteness["formats"].(map[string]any); ok {
			var low []string
			for _, f := range formats {
				if v, ok := fmAny[f].(map[string]any); ok {
					st, _ := v["status"].(string)
					if st == "missing" || st == "stale" {
						minExpected := 1
						if m64, ok := v["expected_min_30d"].(float64); ok {
							minExpected = int(m64)
						}
						n := 0
						if n64, ok := v["matches_last_30d"].(float64); ok {
							n = int(n64)
						}
						low = append(low, fmt.Sprintf("%s (%d<%d)", f, n, minExpected))
					}
				}
			}
			if len(low) > 0 {
				sort.Strings(low)
				out = append(out, map[string]any{
					"reason": fmt.Sprintf(
						"Few or no matches in last 30 days for %s. Consider re-importing or verifying schedule.",
						strings.Join(low, ", "),
					),
					"commands": []string{"make cricsheet-import"},
				})
			}
		}
	}

	return out
}
