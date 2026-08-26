// Package pipeline provides shared orchestration for pipeline steps (import, precompute, export).
// Both HTTP handlers and CLI commands use these helpers to minimize logic duplication.
// Concurrency is per lane: steps sharing a lane run one at a time (see steps.Lane).
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	steps "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// ErrPipelineBusy is returned when another step in the same lane is already running.
var ErrPipelineBusy = errors.New("another pipeline step is already running")

// LaneBusy reports whether any step sharing a lane with the given command is currently
// IN_PROGRESS. Handlers call it to return 409 before starting a step.
//
// This used to consult a hand-maintained list of commands that had already drifted:
// train-combination-meta was missing from it, so that step could overlap a training
// run despite the lock existing precisely to stop that. The lane now comes from the
// step registry, so the list cannot go stale again.
func LaneBusy(ctx context.Context, command string) (bool, error) {
	registry := steps.Steps()
	lane := registry.LaneForCommand(command)
	return tracking.HasInProgressForAnyCommand(ctx, registry.CommandsInLane(lane))
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

	// Enforce the lane: one step at a time within a lane, lanes free to overlap.
	busy, err := LaneBusy(ctx, jobName)
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
