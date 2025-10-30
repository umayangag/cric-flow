package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// WithTx starts a transaction and runs fn within it. On error the tx is rolled back; otherwise committed.
// It passes a context that is bound to the transaction for convenience.
func WithTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	if Pool == nil {
		return ErrPoolNotInitialized
	}
	tx, err := Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		// In case of panic, ensure rollback to free resources; recover is not handled here
		_ = tx.Rollback(ctx)
	}()
	if err := fn(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ErrPoolNotInitialized is returned when Pool is nil.
var ErrPoolNotInitialized = fmtError("db pool not initialized")

type fmtError string

func (e fmtError) Error() string { return string(e) }
