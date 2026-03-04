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
	ctx := parent
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, timeout)
		defer cancel()
	}

	// Enforce singleton: only one pipeline step for the whole system at a time.
	busy, err := tracking.HasInProgressForAnyCommand(ctx, PipelineCommands)
	if err != nil {
		slog.Warn("pipeline: check for existing run failed",
			slog.String("command", jobName),
			slog.Any("err", err))
	}
	if busy {
		return ErrPipelineBusy
	}

	tracker, tErr := tracking.Start(ctx, jobName, startMeta)
	if tErr != nil {
		slog.Warn("pipeline: tracking start failed",
			slog.String("command", jobName),
			slog.Any("err", tErr))
	}
	if tracker != nil {
		defer func() {
			tracker.CaptureExit(ctx, &runErr, exitMeta)
		}()
		// Log with run_id for cross-service correlation (see docs/observability.md).
		slog.Info("pipeline: job starting",
			slog.String("command", jobName),
			slog.Int("run_id", tracker.ID),
			slog.Duration("timeout", timeout),
			slog.Any("start_meta", startMeta))
	} else {
		slog.Info("pipeline: job starting",
			slog.String("command", jobName),
			slog.Duration("timeout", timeout),
			slog.Any("start_meta", startMeta))
	}
	defer func() {
		if v := recover(); v != nil {
			runErr = fmt.Errorf("panic: %v", v)
			attrs := []any{
				slog.String("panic", fmt.Sprint(v)),
				slog.String("stack", string(debug.Stack())),
				slog.String("command", jobName),
			}
			if tracker != nil {
				attrs = append(attrs, slog.Int("run_id", tracker.ID))
			}
			slog.Error("pipeline: job panic (DB 'connection to client lost' usually follows)", attrs...)
		}
	}()

	exitMeta, runErr = fn(ctx)
	if runErr != nil {
		attrs := []any{slog.Any("err", runErr), slog.String("command", jobName), slog.Any("start_meta", startMeta)}
		if tracker != nil {
			attrs = append(attrs, slog.Int("run_id", tracker.ID))
		}
		slog.Error("pipeline: job function failed", attrs...)
	}
	return runErr
}
