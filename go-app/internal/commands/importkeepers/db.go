package importkeepers

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// DB abstracts minimal database operations required by Runner.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
	QueryRowCount(ctx context.Context, sql string, args ...any) (int64, error)
}

// pgAdapter implements DB using internal/db.Pool.
type pgAdapter struct{}

func NewDB() DB { return pgAdapter{} }

func (pgAdapter) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	ct, err := db.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func (pgAdapter) QueryRowCount(ctx context.Context, sql string, args ...any) (int64, error) {
	var n int64
	if err := db.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
