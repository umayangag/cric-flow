package opsstatus

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
	"train_innings":          "train-innings",
	"train_combination_meta": "train-combination-meta",
	"auto_tune":              "ml-auto-tune",
}

// pipelineStepPreviousCommand defines the run order.
var pipelineStepPreviousCommand = map[string]string{
	"import":                 "",
	"precompute":             "cricsheet-import",
	"export":                 "precompute-features",
	"train_batting":          "export-dataset",
	"train_bowling":          "export-dataset",
	"train_fielding":         "export-dataset",
	"train_extras":           "export-dataset",
	"train_win":              "export-dataset",
	"train_innings":          "export-dataset",
	"train_combination_meta": "train-win",
	"auto_tune":              "train-fielding",
}

// BuildPipelineSection returns a map with "steps" (per-step running, runnable) for /ops/status.
func BuildPipelineSection(ctx context.Context) map[string]any {
	inProgress, err := tracking.InProgressByCommand(ctx)
	if err != nil {
		slog.Warn("pipeline: InProgressByCommand failed", "err", err)
		inProgress = map[string]bool{}
	}

	steps := map[string]any{}
	for stepID, command := range pipelineStepCommands {
		running := inProgress[command]
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
		if stepID == "import" {
			runnable = !running
		}
		steps[stepID] = map[string]any{"running": running, "runnable": runnable, "completed": completed}
	}

	autoTuneRunnable := true
	for _, cmd := range []string{"train-fielding", "train-extras", "train-win"} {
		done, err := tracking.HasCompletedSuccessfullyForCommand(ctx, cmd)
		if err != nil || !done {
			autoTuneRunnable = false
			break
		}
	}
	autoTuneRunning := inProgress["ml-auto-tune"]
	autoTuneCompleted, err := tracking.HasCompletedSuccessfullyForCommand(ctx, "ml-auto-tune")
	if err != nil {
		slog.Warn(
			"pipeline: HasCompletedSuccessfullyForCommand failed",
			"step",
			"auto_tune",
			"command",
			"ml-auto-tune",
			"err",
			err,
		)
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
	"train-innings":       "Train Innings",
}

// CanRunPipelineStep returns whether the step can be started and an error message if not.
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
