package importretired

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// DB is a minimal interface for executing write statements.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
}

// pgDB adapts internal/db.Pool to DB.
type pgDB struct{}

// NewDB returns a DB backed by the global connection pool.
func NewDB() DB { return pgDB{} }

func (pgDB) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	ct, err := db.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}
