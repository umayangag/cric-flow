package server

import (
	"context"
	"log/slog"

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

// buildPipelineSection returns a map of pipeline step id -> { "running": bool }
// for use in /ops/status. Running is true when data_migrations has an IN_PROGRESS
// row for that step's command.
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
	// auto_tune is optional and not tracked
	steps["auto_tune"] = map[string]any{"running": false}
	return map[string]any{"steps": steps}
}
