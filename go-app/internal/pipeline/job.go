// Package pipeline provides shared orchestration for pipeline steps (import, precompute, export).
// Both HTTP handlers and CLI commands use these helpers to minimize logic duplication.
// Pipeline steps are singleton: only one step may run across the whole system at a time.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// PipelineCommands are the data_migrations command names that form the main pipeline.
// Only one of these may be IN_PROGRESS at a time (enforced in RunJob and in ml-service Tracker).
var PipelineCommands = []string{
	"cricsheet-import",
	"precompute-features",
	"export-dataset",
	"train-batting",
	"train-bowling",
	"train-fielding",
	"train-extras",
	"train-win",
	"train-innings",
	"ml-auto-tune",
}

// ErrPipelineBusy is returned when another pipeline step is already running (singleton).
var ErrPipelineBusy = errors.New("another pipeline step is already running")

// HasPipelineBusy returns true if any pipeline step is currently IN_PROGRESS.
// Handlers use this to return 409 before starting a new step.
func HasPipelineBusy(ctx context.Context) (bool, error) {
	return tracking.HasInProgressForAnyCommand(ctx, PipelineCommands)
}

// JobFunc runs a pipeline step. It returns (exitMeta, err). On success, exitMeta is
// passed to tracking.CaptureExit; on failure, err is used.
type JobFunc func(ctx context.Context) (exitMeta any, err error)

// RunJob executes a pipeline step with panic recovery, tracking, and optional timeout.
// Callers (handlers and CLI) pass a parent context; timeout is applied on top of it.
// If timeout <= 0, no additional timeout is applied.
func RunJob(parent context.Context, jobName string, startMeta any, timeout time.Duration, fn JobFunc) error {
	var runErr error
	var exitMeta any

	slog.Info("pipeline: job starting",
		slog.String("job", jobName),
		slog.Duration("timeout", timeout),
		slog.Any("start_meta", startMeta))

	ctx := parent
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, timeout)
		defer cancel()
	}

	// Enforce singleton: only one pipeline step for the whole system at a time.
	busy, err := tracking.HasInProgressForAnyCommand(ctx, PipelineCommands)
	if err != nil {
		slog.Warn(jobName+": check for existing pipeline failed", slog.Any("err", err))
	}
	if busy {
		return ErrPipelineBusy
	}

	tracker, tErr := tracking.Start(ctx, jobName, startMeta)
	if tErr != nil {
		slog.Warn(jobName+": tracking start failed", slog.Any("err", tErr))
	}
	if tracker != nil {
		defer func() {
			tracker.CaptureExit(ctx, &runErr, exitMeta)
		}()
	}
	defer func() {
		if v := recover(); v != nil {
			runErr = fmt.Errorf("panic: %v", v)
			slog.Error(jobName+" panic (DB 'connection to client lost' usually follows)",
				slog.String("panic", fmt.Sprint(v)),
				slog.String("stack", string(debug.Stack())))
		}
	}()

	exitMeta, runErr = fn(ctx)
	if runErr != nil {
		slog.Error(jobName+" job function failed",
			slog.Any("err", runErr),
			slog.Any("start_meta", startMeta))
	}
	return runErr
}
