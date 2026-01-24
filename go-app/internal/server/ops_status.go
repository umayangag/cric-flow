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
    Fielding  map[string]any         `json:"fielding"`
    Weather   map[string]any         `json:"weather"`
    Suggestions []map[string]any     `json:"suggestions"`
}

// getPrecomputeStatus is a function variable to allow test-time substitution.
// In production it points to precompute.GetStatus.
var getPrecomputeStatus = precompute.GetStatus

// opsStatusHandler assembles and returns the ops status payload.
func (a *App) opsStatusHandler(w http.ResponseWriter, r *http.Request) {
    resp := a.assembleOpsStatusResponse(r.Context())
    respondJSON(w, http.StatusOK, resp)
}

// assembleOpsStatusResponse constructs the OpsStatusResponse from available sources.
// This function exists to keep the handler concise and the logic easy to read and test.
func (a *App) assembleOpsStatusResponse(ctx context.Context) OpsStatusResponse {
    now := time.Now().UTC()
    resp := OpsStatusResponse{
        Timestamp:  now.Format(time.RFC3339),
        // api_readiness should reflect DB connectivity; initialize to false and
        // update after the DB section is built.
        Services:   map[string]bool{"api_health": true, "api_readiness": false, "ml_health": false},
        DB:         map[string]any{"connected": false},
        Precompute: buildPrecomputeSection(now),
        Exports:    map[string]any{"root": "output/go-app", "formats": map[string]any{}},
        Artifacts:  map[string]any{"root": "output/ml-service", "formats": map[string]any{}},
        Fielding:  map[string]any{},
        Weather:   map[string]any{},
        Suggestions: []map[string]any{},
    }

    // DB
    if a != nil && a.dbProbe != nil {
        resp.DB = buildDBSection(ctx, a.dbProbe)
    }
    // Update api_readiness based on DB connectivity per documentation.
    if connected, ok := resp.DB["connected"].(bool); ok {
        resp.Services["api_readiness"] = connected
    }
    // Exports
    resp.Exports = buildExportsSection("output/go-app")
    // Fielding & Weather (DB-backed counts)
    resp.Fielding = buildFieldingSection(ctx, a.dbProbe)
    resp.Weather = buildWeatherSection(ctx, a.dbProbe)
    // Artifacts + ML health
    if sec, mlOK := buildArtifactsSection(nil, "output/ml-service"); sec != nil {
        resp.Artifacts = sec
        resp.Services["ml_health"] = mlOK
    }
    // Suggestions
    resp.Suggestions = computeSuggestions(resp.DB, resp.Precompute, resp.Exports, resp.Artifacts, resp.Services, resp.Fielding, resp.Weather)
    return resp
}

// buildPrecomputeSection constructs the precompute part of the ops status based on
// the in-memory status from the precompute package and a provided current time.
// Freshness rule: a format is "ok" if FinishedAt is on the same UTC date as now
// and the format was included in the last run; otherwise it is "stale". If no
// successful run (FinishedAt zero), all formats are "missing".
func buildPrecomputeSection(now time.Time) map[string]any {
    stat := getPrecomputeStatus()
    // Supported cricket formats
    formats := getCricketFormats()

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
