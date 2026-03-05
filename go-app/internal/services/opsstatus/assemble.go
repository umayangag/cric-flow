package opsstatus

import (
	"context"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// AssembleResponse constructs the full Response from available sources.
func AssembleResponse(ctx context.Context, dbProbe DBProbe) Response {
	now := time.Now().UTC()
	resp := Response{
		Timestamp:      now.Format(time.RFC3339),
		Services:       map[string]bool{"api_health": true, "api_readiness": false, "ml_health": false},
		DB:             map[string]any{"connected": false},
		Precompute:     BuildPrecomputeSection(ctx, now),
		Exports:        map[string]any{"root": config.DefaultExportDir(), "formats": map[string]any{}},
		Artifacts:      map[string]any{"root": ArtifactsFallbackRoot(), "formats": map[string]any{}},
		Fielding:       map[string]any{},
		Weather:        map[string]any{},
		DBFreshness:    map[string]any{},
		DBCompleteness: map[string]any{},
	}

	// DB
	if dbProbe != nil {
		resp.DB = BuildDBSection(ctx, dbProbe)
	}
	if connected, ok := resp.DB["connected"].(bool); ok {
		resp.Services["api_readiness"] = connected
	}
	// Exports
	resp.Exports = BuildExportsSection(config.DefaultExportDir())
	// Fielding & Weather
	resp.Fielding = BuildFieldingSection(ctx, dbProbe)
	resp.Weather = BuildWeatherSection(ctx, dbProbe)
	// DB insights
	resp.DBFreshness = BuildDBFreshnessSection(ctx, NewProductionInsightsProbe(), now)
	resp.DBCompleteness = BuildDBCompletenessSection(ctx, NewProductionInsightsProbe(), now)
	// Artifacts + ML health
	if sec, mlOK := BuildArtifactsSection(nil, ArtifactsFallbackRoot()); sec != nil {
		resp.Artifacts = sec
		resp.Services["ml_health"] = mlOK
	}
	resp.Pipeline = BuildPipelineSection(ctx)
	return resp
}
