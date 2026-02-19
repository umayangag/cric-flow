package server

import (
	"context"
	"log/slog"

	"github.com/umayangag/cric-flow/go-app/internal/tracking"
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

// getPipelineCommands returns the list of pipeline command names for tracking (in progress / runnable checks).
func getPipelineCommands() []string {
	commands := make([]string, 0, len(pipelineStepCommands))
	for _, cmd := range pipelineStepCommands {
		commands = append(commands, cmd)
	}
	return commands
}

// pipelineStepOrder defines the run order; step N is only runnable after step N-1 completed successfully.
// Empty string means no previous step (always runnable when no other pipeline is running).
var pipelineStepPreviousCommand = map[string]string{
	"import":         "", // first step
	"precompute":     "cricsheet-import",
	"export":         "precompute-features",
	"train_batting":  "export-dataset",
	"train_bowling":  "export-dataset",
	"train_fielding": "export-dataset",
	"auto_tune":      "train-fielding", // optional; only after training completed
}

// buildPipelineSection returns a map with "steps" (per-step running, runnable) for /ops/status.
// A step is runnable only when no pipeline is running and the previous step has completed successfully.
func buildPipelineSection(ctx context.Context) map[string]any {
	anyRunning, _ := tracking.HasInProgressForAnyCommand(ctx, getPipelineCommands())
	steps := map[string]any{}
	for stepID, command := range pipelineStepCommands {
		running, err := tracking.HasInProgressForCommand(ctx, command)
		if err != nil {
			slog.Warn("pipeline: HasInProgressForCommand failed", "step", stepID, "command", command, "err", err)
			running = false
		}
		runnable := !anyRunning && !running
		if runnable {
			prevCmd := pipelineStepPreviousCommand[stepID]
			if prevCmd != "" {
				prevDone, err := tracking.HasCompletedSuccessfullyForCommand(ctx, prevCmd)
				if err != nil {
					slog.Warn(
						"pipeline: HasCompletedSuccessfullyForCommand failed",
						"step",
						stepID,
						"prev",
						prevCmd,
						"err",
						err,
					)
					runnable = false
				} else {
					runnable = prevDone
				}
			}
		}
		steps[stepID] = map[string]any{"running": running, "runnable": runnable}
	}
	// auto_tune: runnable only when no pipeline running and train-fielding has completed (optional step)
	autoTuneRunnable := !anyRunning
	if autoTuneRunnable {
		done, err := tracking.HasCompletedSuccessfullyForCommand(ctx, "train-fielding")
		if err != nil {
			autoTuneRunnable = false
		} else {
			autoTuneRunnable = done
		}
	}
	steps["auto_tune"] = map[string]any{"running": false, "runnable": autoTuneRunnable}
	return map[string]any{"steps": steps}
}

// stepLabelByCommand returns a short label for the previous step (for error messages).
var stepLabelByCommand = map[string]string{
	"cricsheet-import":    "Import",
	"precompute-features": "Precompute",
	"export-dataset":      "Export",
	"train-fielding":      "Train Fielding",
}

// CanRunPipelineStep returns whether the step can be started and an error message if not.
// Used by pipeline run handler to enforce order: next step only after previous completed successfully.
func CanRunPipelineStep(ctx context.Context, stepID string) (ok bool, errMsg string) {
	_, hasCommand := pipelineStepCommands[stepID]
	if !hasCommand && stepID != "auto_tune" {
		return false, "unknown step"
	}
	anyRunning, _ := tracking.HasInProgressForAnyCommand(ctx, getPipelineCommands())
	if anyRunning {
		return false, "another pipeline step is already running"
	}
	prevCmd := pipelineStepPreviousCommand[stepID]
	if prevCmd == "" {
		return true, ""
	}
	prevDone, err := tracking.HasCompletedSuccessfullyForCommand(ctx, prevCmd)
	if err != nil {
		return false, "could not verify previous step"
	}
	if !prevDone {
		label := stepLabelByCommand[prevCmd]
		if label == "" {
			label = prevCmd
		}
		return false, "complete the previous step (" + label + ") first"
	}
	return true, ""
}
