package server

import (
    "context"
    "net/http"
    "time"

    "github.com/umayangag/cric-info-scrapers/go-app/internal/precompute"
)

// OpsStatusResponse is the top-level JSON returned by /ops/status.
type OpsStatusResponse struct {
    Timestamp string                 `json:"timestamp"`
    Services  map[string]bool        `json:"services"`
    DB        map[string]any         `json:"db"`
    Precompute map[string]any        `json:"precompute"`
    Exports   map[string]any         `json:"exports"`
    Artifacts map[string]any         `json:"artifacts"`
    Suggestions []map[string]any     `json:"suggestions"`
}

// getPrecomputeStatus is a function variable to allow test-time substitution.
// In production it points to precompute.GetStatus.
var getPrecomputeStatus = precompute.GetStatus

// opsStatusHandler returns a scaffolded ops status payload.
// Subsequent steps will populate real values and wire dependencies.
func (a *App) opsStatusHandler(w http.ResponseWriter, r *http.Request) {
    now := time.Now().UTC().Format(time.RFC3339)
    // Default response scaffold
    resp := OpsStatusResponse{
        Timestamp: now,
        Services: map[string]bool{
            "api_health":    true,
            "api_readiness": true,
            "ml_health":     false,
        },
        DB: map[string]any{"connected": false},
        Precompute: buildPrecomputeSection(time.Now().UTC()),
        Exports: map[string]any{"root": "output/go-app", "formats": map[string]any{}},
        Artifacts: map[string]any{"root": "output/ml-service", "formats": map[string]any{}},
        Suggestions: []map[string]any{},
    }

    // Populate DB section using probe (if available)
    if a != nil && a.dbProbe != nil {
        ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
        defer cancel()
        if err := a.dbProbe.Ping(ctx); err == nil {
            resp.DB["connected"] = true
            // Counts: players, matches, innings
            counts := map[string]int64{}
            if n, err := a.dbProbe.Count(ctx, "players"); err == nil { counts["players"] = n }
            if n, err := a.dbProbe.Count(ctx, "matches"); err == nil { counts["matches"] = n }
            if n, err := a.dbProbe.Count(ctx, "innings"); err == nil { counts["innings"] = n }
            resp.DB["counts"] = counts

            if cur, exp, status, err := a.dbProbe.MigrationInfo(ctx); err == nil {
                resp.DB["migration"] = map[string]any{"status": status, "current": cur, "expected": exp}
            } else {
                resp.DB["migration"] = map[string]any{"status": "unknown"}
            }
        } else {
            // disconnected: keep connected=false, but include expected if known
            if _, exp, status, _ := a.dbProbe.MigrationInfo(ctx); exp > 0 {
                resp.DB["migration"] = map[string]any{"status": status, "expected": exp}
            } else {
                resp.DB["migration"] = map[string]any{"status": "unknown"}
            }
        }
    }
    // Populate exports section from filesystem under output/go-app
    resp.Exports = buildExportsSection("output/go-app")

    // Populate artifacts section via ML HTTP + FS fallback under output/ml-service
    if sec, mlOK := buildArtifactsSection(nil, "output/ml-service"); sec != nil {
        resp.Artifacts = sec
        resp.Services["ml_health"] = mlOK
    }
    // Compute suggestions from the assembled snapshot
    resp.Suggestions = computeSuggestions(resp.DB, resp.Precompute, resp.Exports, resp.Artifacts, resp.Services)

    respondJSON(w, http.StatusOK, resp)
}

// buildPrecomputeSection constructs the precompute part of the ops status based on
// the in-memory status from the precompute package and a provided current time.
// Freshness rule: a format is "ok" if FinishedAt is on the same UTC date as now
// and the format was included in the last run; otherwise it is "stale". If no
// successful run (FinishedAt zero), all formats are "missing".
func buildPrecomputeSection(now time.Time) map[string]any {
    stat := getPrecomputeStatus()
    // Supported cricket formats
    formats := []string{"TEST", "ODI", "T20I", "T20"}

    section := map[string]any{
        "last_run": "",
        "as_of":    "",
        "formats":  map[string]any{},
    }

    fm := map[string]any{}
    for _, f := range formats {
        fm[f] = map[string]any{"status": "missing"}
    }

    // If we have a finished run time, compute freshness
    if !stat.FinishedAt.IsZero() {
        section["last_run"] = stat.FinishedAt.UTC().Format(time.RFC3339)
        section["as_of"] = stat.FinishedAt.UTC().Format("2006-01-02")

        // Build a quick lookup for formats included in last run
        ran := map[string]struct{}{}
        for _, f := range stat.Formats {
            ran[f] = struct{}{}
        }
        // Same UTC day helper
        y1, m1, d1 := now.UTC().Date()
        y2, m2, d2 := stat.FinishedAt.UTC().Date()
        sameDay := (y1 == y2 && m1 == m2 && d1 == d2)

        for _, f := range formats {
            if _, ok := ran[f]; ok {
                if sameDay {
                    fm[f] = map[string]any{"status": "ok"}
                } else {
                    fm[f] = map[string]any{"status": "stale"}
                }
            } else {
                // not part of last run => missing
                fm[f] = map[string]any{"status": "missing"}
            }
        }
    }

    section["formats"] = fm
    return section
}
