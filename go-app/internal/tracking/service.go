package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

type Tracker struct {
	ID int
}

// Sentinels StartExclusive returns instead of a tracker.
var (
	// ErrRunConflict means another run already holds the exclusivity set and this one
	// must not start.
	ErrRunConflict = errors.New("another run is already in progress")
	// ErrNoDatabase means there is nowhere to record the run, so there is nothing to
	// claim either. Callers decide whether that is fatal; the API's own unit paths run
	// without a pool.
	ErrNoDatabase = errors.New("no database is configured to record the run in")
)

// advisoryLockClass namespaces cric-flow's Postgres advisory locks in the upper half
// of the 64-bit key space, so a key derived from a lane name cannot collide with some
// other use of advisory locks on the same database.
const advisoryLockClass int64 = 0x63726963 // "cric"

// advisoryLockFor places a caller's key inside that namespace.
func advisoryLockFor(key uint32) int64 {
	return advisoryLockClass<<32 | int64(key)
}

// claimTimeout bounds the whole claim. The advisory lock is held only for the length
// of this one small transaction, so waiting on another process's claim is a matter of
// milliseconds; anything approaching this timeout is a database in trouble.
const claimTimeout = 10 * time.Second

// StartExclusive records the start of a run, but only if no run in conflicting is
// already IN_PROGRESS. It returns ErrRunConflict when one is.
//
// The check and the insert happen inside one transaction that first takes a Postgres
// advisory lock on lockKey, which is what makes them atomic. They used to be two
// separate statements with nothing between them (GO-06): two POSTs arriving within one
// round trip both read "not busy", both inserted, and both ran — with the in-memory
// cancel registry holding only the second, so Stop cancelled neither.
//
// The lock is in the database rather than in this process on purpose. A `sync.Mutex`
// would serialise the API's own handlers and nothing else, and the API is not the only
// writer: `cmd/cricsheet-importer` calls pipeline.RunJob in a separate process against
// the same database. An advisory lock serialises every claimant of the same key
// whatever process it is in, and being transaction-scoped it cannot be leaked by a
// pooled connection that never comes back to release it.
func StartExclusive(
	ctx context.Context,
	command string,
	args any,
	lockKey uint32,
	conflicting []string,
) (*Tracker, error) {
	if !db.Available() {
		return nil, ErrNoDatabase
	}

	// Detached from the caller's context, as this has been since it was Start: a run
	// whose context is already on its way out still has to leave a row behind, or the
	// run exists with nothing recording it.
	claimCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), claimTimeout)
	defer cancel()

	tx, err := db.Begin(claimCtx)
	if err != nil {
		return nil, fmt.Errorf("begin the run claim: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(claimCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("tracking: rolling back the run claim failed", "err", rollbackErr)
		}
	}()

	if err := tx.Exec(claimCtx, `SELECT pg_advisory_xact_lock($1)`, advisoryLockFor(lockKey)); err != nil {
		return nil, fmt.Errorf("take the run lock: %w", err)
	}

	busy, err := anyInProgressTx(claimCtx, tx, conflicting)
	if err != nil {
		return nil, fmt.Errorf("check for a run already in flight: %w", err)
	}
	if busy {
		return nil, ErrRunConflict
	}

	var id int
	err = tx.QueryRow(claimCtx, `
		INSERT INTO data_migrations (command, args, status, started_at)
		VALUES ($1, $2, $3, NOW())
		RETURNING id
	`, command, marshalRunArgs(args), StatusInProgress).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("record the run start: %w", err)
	}
	if err := tx.Commit(claimCtx); err != nil {
		return nil, fmt.Errorf("commit the run claim: %w", err)
	}
	return &Tracker{ID: id}, nil
}

// anyInProgressTx answers the lane question inside the claim's transaction, so it is
// asked under the advisory lock rather than before it.
func anyInProgressTx(ctx context.Context, tx db.Tx, commands []string) (bool, error) {
	if len(commands) == 0 {
		return false, nil
	}
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM data_migrations
			WHERE status = $1 AND command = ANY($2::text[])
		)
	`, StatusInProgress, commands).Scan(&exists)
	return exists, err
}

// marshalRunArgs encodes what a run was asked to do. Args that will not marshal are
// recorded as an empty object rather than failing the run: the row matters more than
// its arguments.
func marshalRunArgs(args any) json.RawMessage {
	encoded, err := json.Marshal(args)
	if err != nil {
		slog.Error("failed to marshal args", "err", err)
		return json.RawMessage("{}")
	}
	return encoded
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
