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
		DBCompleteness: map[string]any{},
	}
	insights := NewProductionInsightsProbe()

	// DB
	if dbProbe != nil {
		resp.DB = BuildDBSection(ctx, dbProbe)
	}
	if connected, ok := resp.DB["connected"].(bool); ok {
		resp.Services["api_readiness"] = connected
	}
	// Whether renamed clubs have been joined back up is a fact about the archive, not
	// about the last import, so it is read here rather than trusted from a run's log: an
	// import that aborted before settlement wrote nothing and logged nothing (IMPORT-07).
	resp.DB["team_lineage"] = BuildTeamLineageSection(ctx, NewProductionTeamLineageProbe())
	resp.Fielding = BuildFieldingSection(ctx, dbProbe)
	resp.DBCompleteness = BuildDBCompletenessSection(ctx, insights, now)
	// Artifacts + ML health
	if sec, mlOK := BuildArtifactsSection(nil, ArtifactsFallbackRoot()); sec != nil {
		resp.Artifacts = sec
		resp.Services["ml_health"] = mlOK
	}
	// Freshness is assembled after the artifacts because it copies H-11's verdict out of
	// them: go-app sees both the database and ml-service, and this is the one place the
	// two are put side by side (P2-1).
	resp.Freshness = BuildFreshnessSection(ctx, insights, resp.Artifacts, now)
	resp.Pipeline = BuildPipelineSection(ctx)
	return resp
}
