// Package pipeline provides shared orchestration for pipeline steps (import, precompute, export).
// Both HTTP handlers and CLI commands use these helpers to minimize logic duplication.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
)

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
