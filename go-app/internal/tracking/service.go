package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"
)

type Tracker struct {
	ID int
}

func Start(_ context.Context, command string, args any) (*Tracker, error) {
	// Use background context for start so it doesn't fail if ctx is canceled (e.g. timeout during initialization)
	updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	argsBytes, err := json.Marshal(args)
	if err != nil {
		slog.Error("failed to marshal args", "err", err)
		argsBytes = []byte("{}")
	}

	id, err := CreateMigration(updateCtx, command, argsBytes)
	if err != nil {
		return nil, err
	}
	return &Tracker{ID: id}, nil
}

func (t *Tracker) Complete(ctx context.Context, metadata any) error {
	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		slog.Error("failed to marshal metadata", "err", err)
		metaBytes = []byte("{}")
	}
	return UpdateMigrationStatus(ctx, t.ID, StatusCompleted, metaBytes, "")
}

func (t *Tracker) Fail(ctx context.Context, errStr string) error {
	return UpdateMigrationStatus(ctx, t.ID, StatusFailed, nil, errStr)
}

func (t *Tracker) Cancel(ctx context.Context) error {
	return UpdateMigrationStatus(ctx, t.ID, StatusCancelled, nil, "")
}

func (t *Tracker) TryComplete(ctx context.Context, metadata any) {
	if t == nil {
		return
	}
	if err := t.Complete(ctx, metadata); err != nil {
		slog.Warn("failed to update tracking status to COMPLETED", "id", t.ID, "err", err)
	}
}

func (t *Tracker) TryFail(ctx context.Context, errStr string) {
	if t == nil {
		return
	}
	if err := t.Fail(ctx, errStr); err != nil {
		slog.Warn("failed to update tracking status to FAILED", "id", t.ID, "err", err)
	}
}

// CaptureExit is intended to be used with defer to automatically update the
// tracking status based on the error pointer and context state.
func (t *Tracker) CaptureExit(ctx context.Context, errPtr *error, metadata any) {
	if t == nil {
		return
	}

	// Use a fresh background context for the update to ensure it completes
	// even if the original context was cancelled.
	updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if errPtr != nil && *errPtr != nil {
		if errors.Is(*errPtr, context.Canceled) || errors.Is(*errPtr, context.DeadlineExceeded) {
			if err := t.Cancel(updateCtx); err != nil {
				slog.Warn("failed to update tracking status to CANCELLED", "id", t.ID, "err", err)
			}
			return
		}
		if err := t.Fail(updateCtx, (*errPtr).Error()); err != nil {
			slog.Warn("failed to update tracking status to FAILED", "id", t.ID, "err", err)
		}
		return
	}

	// Check if context was cancelled even if no error was returned
	if ctx.Err() != nil {
		if err := t.Cancel(updateCtx); err != nil {
			slog.Warn("failed to update tracking status to CANCELLED", "id", t.ID, "err", err)
		}
		return
	}

	if err := t.Complete(updateCtx, metadata); err != nil {
		slog.Warn("failed to update tracking status to COMPLETED", "id", t.ID, "err", err)
	}
}
