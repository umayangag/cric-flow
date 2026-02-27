package db_test

import (
	"context"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// mockDB adapts pgxmock.PgxPoolIface to db.DB for tests (keeper_repo, repo_backtest_features, repo_match_query).
type mockDB struct {
	pool pgxmock.PgxPoolIface
}

func (m mockDB) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := m.pool.Exec(ctx, sql, args...)
	return err
}

func (m mockDB) Query(ctx context.Context, sql string, args ...any) (db.Rows, error) {
	r, err := m.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &mockRows{Rows: r}, nil
}

func (m mockDB) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	return &mockRow{Row: m.pool.QueryRow(ctx, sql, args...)}
}

func (m mockDB) Begin(ctx context.Context) (db.Tx, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &mockTxImpl{tx: tx}, nil
}

type mockRows struct{ pgx.Rows }

func (r *mockRows) Close() { r.Rows.Close() }

type mockRow struct{ pgx.Row }

func (r *mockRow) Scan(dest ...any) error { return r.Row.Scan(dest...) }

type mockTxImpl struct {
	tx pgx.Tx
}

func (t *mockTxImpl) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := t.tx.Exec(ctx, sql, args...)
	return err
}

func (t *mockTxImpl) Query(ctx context.Context, sql string, args ...any) (db.Rows, error) {
	r, err := t.tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &mockRows{Rows: r}, nil
}

func (t *mockTxImpl) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	return &mockRow{Row: t.tx.QueryRow(ctx, sql, args...)}
}

func (t *mockTxImpl) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *mockTxImpl) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

// optionsStringRows implements db.Rows for repo_options tests.
type optionsStringRows struct {
	vals    []string
	idx     int
	err     error
	scanErr error
}

func stringRowsForOptions(vals []string) db.Rows {
	return &optionsStringRows{vals: vals}
}

func (r *optionsStringRows) Next() bool {
	if r.idx < len(r.vals) {
		r.idx++
		return true
	}
	return false
}

func (r *optionsStringRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.idx == 0 || r.idx > len(r.vals) || len(dest) == 0 {
		return nil
	}
	if p, ok := dest[0].(*string); ok {
		*p = r.vals[r.idx-1]
		return nil
	}
	return nil
}

func (r *optionsStringRows) Close() {}
func (r *optionsStringRows) Err() error { return r.err }
