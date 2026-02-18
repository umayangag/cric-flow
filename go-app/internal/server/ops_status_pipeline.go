package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
)

// Pipeline step IDs and their corresponding data_migrations command names.
var pipelineStepCommands = map[string]string{
	"import":         "cricsheet-import",
	"precompute":     "precompute-features",
	"export":         "export-dataset",
	"train_batting":  "train-batting",
	"train_bowling":  "train-bowling",
	"train_fielding": "train-fielding",
	// auto_tune has no tracking command; optional step
}

const pipelineRecentLimit = 5

// buildPipelineSection returns a map with "steps" (per-step running bool) and "overview"
// (in_progress jobs and recent completed/failed) for /ops/status.
func buildPipelineSection(ctx context.Context) map[string]any {
	steps := map[string]any{}
	for stepID, command := range pipelineStepCommands {
		running, err := tracking.HasInProgressForCommand(ctx, command)
		if err != nil {
			slog.Warn("pipeline: HasInProgressForCommand failed", "step", stepID, "command", command, "err", err)
			running = false
		}
		steps[stepID] = map[string]any{"running": running}
	}
	steps["auto_tune"] = map[string]any{"running": false}

	inProgress, _ := tracking.GetInProgressMigrations(ctx)
	recent, _ := tracking.GetRecentMigrations(ctx, pipelineRecentLimit)

	overview := map[string]any{
		"in_progress": buildInProgressOverview(inProgress),
		"recent":      buildRecentOverview(recent),
	}
	return map[string]any{"steps": steps, "overview": overview}
}

func buildInProgressOverview(migrations []tracking.Migration) []map[string]any {
	out := make([]map[string]any, 0, len(migrations))
	for _, m := range migrations {
		out = append(out, map[string]any{
			"id":         m.ID,
			"command":   m.Command,
			"started_at": formatTime(m.StartedAt),
		})
	}
	return out
}

func buildRecentOverview(migrations []tracking.Migration) []map[string]any {
	out := make([]map[string]any, 0, len(migrations))
	for _, m := range migrations {
		entry := map[string]any{
			"id":         m.ID,
			"command":   m.Command,
			"started_at": formatTime(m.StartedAt),
			"status":    string(m.Status),
		}
		if m.CompletedAt != nil {
			entry["completed_at"] = formatTime(*m.CompletedAt)
		}
		if m.ErrorMessage != "" {
			entry["error_message"] = m.ErrorMessage
		}
		out = append(out, entry)
	}
	return out
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
