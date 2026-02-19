package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
	"github.com/umayangag/cric-flow/go-app/internal/precompute"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// OpsStatusResponse is the top-level JSON returned by /ops/status.
type OpsStatusResponse struct {
	Timestamp      string                        `json:"timestamp"`
	Services       map[string]bool               `json:"services"`
	DB             map[string]any                `json:"db"`
	Precompute     map[string]any                `json:"precompute"`
	Exports        map[string]any                `json:"exports"`
	Artifacts      map[string]any                `json:"artifacts"`
	Fielding       map[string]any                `json:"fielding"`
	Weather        map[string]any                `json:"weather"`
	DBFreshness    map[string]any                `json:"db_freshness"`
	DBCompleteness map[string]any                `json:"db_completeness"`
	Hierarchy      []formats.FormatHierarchyNode `json:"hierarchy"`
	// Pipeline reports per-step running state (from data_migrations IN_PROGRESS).
	Pipeline map[string]any `json:"pipeline,omitempty"`
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
		Timestamp: now.Format(time.RFC3339),
		// api_readiness should reflect DB connectivity; initialize to false and
		// update after the DB section is built.
		Services:       map[string]bool{"api_health": true, "api_readiness": false, "ml_health": false},
		DB:             map[string]any{"connected": false},
		Precompute:     buildPrecomputeSection(ctx, now),
		Exports:        map[string]any{"root": config.DefaultExportDir(), "formats": map[string]any{}},
		Artifacts:      map[string]any{"root": artifactsFallbackRoot(), "formats": map[string]any{}},
		Fielding:       map[string]any{},
		Weather:        map[string]any{},
		DBFreshness:    map[string]any{},
		DBCompleteness: map[string]any{},
		Hierarchy:      formats.GetHierarchy(),
	}

	// DB
	if a != nil && a.dbProbe != nil {
		resp.DB = buildDBSection(ctx, a.dbProbe)
	}
	// Update api_readiness based on DB connectivity per documentation.
	if connected, ok := resp.DB["connected"].(bool); ok {
		resp.Services["api_readiness"] = connected
	}
	// Exports (use GO_APP_OUTPUT_DIR / config so Docker mount and host paths are correct)
	resp.Exports = buildExportsSection(config.DefaultExportDir())
	// Fielding & Weather (DB-backed counts)
	resp.Fielding = buildFieldingSection(ctx, a.dbProbe)
	resp.Weather = buildWeatherSection(ctx, a.dbProbe)
	// DB insights: freshness & completeness
	resp.DBFreshness = buildDBFreshnessSection(ctx, productionInsightsProbe{}, now)
	resp.DBCompleteness = buildDBCompletenessSection(ctx, productionInsightsProbe{}, now)
	// Artifacts + ML health (HTTP primary; filesystem fallback uses GO_APP_ARTIFACTS_ROOT or default)
	if sec, mlOK := buildArtifactsSection(nil, artifactsFallbackRoot()); sec != nil {
		resp.Artifacts = sec
		resp.Services["ml_health"] = mlOK
	}
	resp.Pipeline = buildPipelineSection(ctx)
	return resp
}

// buildPrecomputeSection constructs the precompute part of the ops status based on
// the in-memory status from the precompute package and a provided current time.
// If in-memory has no finished run (e.g. API restarted or precompute ran via CLI),
// it falls back to the tracking DB: a completed "precompute-features" run sets
// precompute as done so the pipeline UI shows Precompute complete regardless of
// later steps (e.g. Export). Freshness: "ok" if last run is same UTC day, else "stale".
func buildPrecomputeSection(ctx context.Context, now time.Time) map[string]any {
	stat := getPrecomputeStatus()
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

	var finishedAt time.Time
	var formatsRan []string
	if !stat.FinishedAt.IsZero() {
		finishedAt = stat.FinishedAt.UTC()
		formatsRan = stat.Formats
	} else {
		// Fallback: use persisted tracking so Precompute shows complete after restart or CLI run
		lastCompleted, err := tracking.GetLastCompletedAtForCommand(ctx, "precompute-features")
		if err != nil {
			slog.Warn("precompute section: GetLastCompletedAtForCommand failed", "err", err)
		}
		if lastCompleted != nil {
			finishedAt = lastCompleted.UTC()
			formatsRan = formats // tracking has no per-format; treat all as ran
		}
	}

	if !finishedAt.IsZero() {
		section["last_run"] = finishedAt.Format(time.RFC3339)
		section["as_of"] = finishedAt.Format("2006-01-02")

		ran := make(map[string]struct{})
		for _, f := range formatsRan {
			ran[f] = struct{}{}
		}
		y1, m1, d1 := now.UTC().Date()
		y2, m2, d2 := finishedAt.Date()
		sameDay := (y1 == y2 && m1 == m2 && d1 == d2)

		for _, f := range formats {
			if _, ok := ran[f]; ok {
				if sameDay {
					fm[f] = map[string]any{"status": "ok"}
				} else {
					fm[f] = map[string]any{"status": "stale"}
				}
			} else {
				fm[f] = map[string]any{"status": "missing"}
			}
		}
	}

	section["formats"] = fm
	return section
}
