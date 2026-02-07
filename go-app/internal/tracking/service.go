package tracking

import (
	"context"
	"encoding/json"
	"log/slog"
)

type Tracker struct {
	ID int
}

func Start(ctx context.Context, command string, args any) (*Tracker, error) {
	argsBytes, err := json.Marshal(args)
	if err != nil {
		slog.Error("failed to marshal args", "err", err)
		argsBytes = []byte("{}")
	}

	id, err := CreateMigration(ctx, command, argsBytes)
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
