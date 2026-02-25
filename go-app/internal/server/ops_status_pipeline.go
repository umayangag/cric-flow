package server

import (
	"context"
	"log/slog"

	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// Pipeline step IDs and their corresponding data_migrations command names.
var pipelineStepCommands = map[string]string{
	"import":                 "cricsheet-import",
	"precompute":             "precompute-features",
	"export":                 "export-dataset",
	"train_batting":          "train-batting",
	"train_bowling":          "train-bowling",
	"train_fielding":         "train-fielding",
	"train_extras":           "train-extras",
	"train_win":              "train-win",
	"train_combination_meta": "train-combination-meta", // no tracking; optional step
	"auto_tune":              "ml-auto-tune",
}

// pipelineStepOrder defines the run order; step N is only runnable after step N-1 completed successfully.
// Empty string means no previous step (always runnable when no other pipeline is running).
var pipelineStepPreviousCommand = map[string]string{
	"import":                 "", // first step
	"precompute":             "cricsheet-import",
	"export":                 "precompute-features",
	"train_batting":          "export-dataset",
	"train_bowling":          "export-dataset",
	"train_fielding":         "export-dataset",
	"train_extras":           "export-dataset",
	"train_win":              "export-dataset",
	"train_combination_meta": "train-win",      // optional; run after train-win; requires contributions CSV
	"auto_tune":              "train-fielding", // optional; runnable when train-fielding AND train-extras AND train-win all done (checked below)
}

// buildPipelineSection returns a map with "steps" (per-step running, runnable) for /ops/status.
// A step is runnable when it is not running and its previous step has completed successfully.
// Steps that share the same previous (e.g. train_batting, train_bowling, train_fielding after export) can run concurrently.
// Import is always runnable when not running so the pipeline can be retriggered from the beginning.
func buildPipelineSection(ctx context.Context) map[string]any {
	steps := map[string]any{}
	for stepID, command := range pipelineStepCommands {
		running, err := tracking.HasInProgressForCommand(ctx, command)
		if err != nil {
			slog.Warn("pipeline: HasInProgressForCommand failed", "step", stepID, "command", command, "err", err)
			running = false
		}
		completed, err := tracking.HasCompletedSuccessfullyForCommand(ctx, command)
		if err != nil {
			slog.Warn(
				"pipeline: HasCompletedSuccessfullyForCommand failed",
				"step",
				stepID,
				"command",
				command,
				"err",
				err,
			)
			completed = false
		}
		runnable := !running
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
		// Import can always be retriggered to reset the pipeline
		if stepID == "import" {
			runnable = !running
		}
		steps[stepID] = map[string]any{"running": running, "runnable": runnable, "completed": completed}
	}
	// auto_tune: runnable when train-fielding, train-extras, train-win have all completed; running/completed from tracking
	autoTuneRunnable := true
	for _, cmd := range []string{"train-fielding", "train-extras", "train-win"} {
		done, err := tracking.HasCompletedSuccessfullyForCommand(ctx, cmd)
		if err != nil || !done {
			autoTuneRunnable = false
			break
		}
	}
	autoTuneRunning, err := tracking.HasInProgressForCommand(ctx, "ml-auto-tune")
	if err != nil {
		slog.Warn("pipeline: HasInProgressForCommand failed", "step", "auto_tune", "command", "ml-auto-tune", "err", err)
		autoTuneRunning = false
	}
	autoTuneCompleted, err := tracking.HasCompletedSuccessfullyForCommand(ctx, "ml-auto-tune")
	if err != nil {
		slog.Warn("pipeline: HasCompletedSuccessfullyForCommand failed", "step", "auto_tune", "command", "ml-auto-tune", "err", err)
		autoTuneCompleted = false
	}
	steps["auto_tune"] = map[string]any{
		"running":   autoTuneRunning,
		"runnable":  autoTuneRunnable && !autoTuneRunning,
		"completed": autoTuneCompleted,
	}
	return map[string]any{"steps": steps}
}

// stepLabelByCommand returns a short label for the previous step (for error messages).
var stepLabelByCommand = map[string]string{
	"cricsheet-import":    "Import",
	"precompute-features": "Precompute",
	"export-dataset":      "Export",
	"train-fielding":      "Train Fielding",
	"train-extras":        "Train Extras",
	"train-win":           "Train Win",
}

// CanRunPipelineStep returns whether the step can be started and an error message if not.
// Used by pipeline run handler to enforce order: next step only after previous completed successfully.
// Multiple steps that share the same previous (e.g. train_batting, train_bowling) may run concurrently.
// auto_tune requires train-fielding, train-extras, and train-win all completed.
func CanRunPipelineStep(ctx context.Context, stepID string) (ok bool, errMsg string) {
	_, hasCommand := pipelineStepCommands[stepID]
	if !hasCommand && stepID != "auto_tune" {
		return false, "unknown step"
	}
	command := pipelineStepCommands[stepID]
	running, err := tracking.HasInProgressForCommand(ctx, command)
	if err != nil {
		return false, "could not verify if step is running"
	}
	if running {
		return false, "this step is already running"
	}
	if stepID == "auto_tune" {
		for _, cmd := range []string{"train-fielding", "train-extras", "train-win"} {
			done, err := tracking.HasCompletedSuccessfullyForCommand(ctx, cmd)
			if err != nil {
				return false, "could not verify previous step"
			}
			if !done {
				label := stepLabelByCommand[cmd]
				if label == "" {
					label = cmd
				}
				return false, "complete " + label + " first (and all API-based training steps)"
			}
		}
		return true, ""
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
