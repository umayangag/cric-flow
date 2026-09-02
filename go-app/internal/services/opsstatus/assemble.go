package opsstatus

import (
	"context"
	"time"
)

// AssembleResponse constructs the full Response from available sources.
func AssembleResponse(ctx context.Context, dbProbe DBProbe) Response {
	now := time.Now().UTC()
	resp := Response{
		Timestamp:      now.Format(time.RFC3339),
		Services:       map[string]bool{"api_health": true, "api_readiness": false, "ml_health": false},
		DB:             map[string]any{"connected": false},
		Dataset:        BuildDatasetSection(),
		Artifacts:      map[string]any{"root": ArtifactsFallbackRoot(), "runs": []map[string]any{}},
		Fielding:       map[string]any{},
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
	resp.Fielding = BuildFieldingSection(ctx, dbProbe)
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
