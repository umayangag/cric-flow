package db_test

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// mockDB adapts pgxmock pool to our DB interface for tests.
type mockDB struct{ pool pgxmock.PgxPoolIface }

type mockRows struct{ rows pgx.Rows }

func (r mockRows) Next() bool             { return r.rows.Next() }
func (r mockRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r mockRows) Close()                 { r.rows.Close() }

type mockRow struct{ row pgx.Row }

func (r mockRow) Scan(dest ...any) error { return r.row.Scan(dest...) }

func (m mockDB) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := m.pool.Exec(ctx, sql, args...)
	return err
}

func (m mockDB) Query(ctx context.Context, sql string, args ...any) (db.Rows, error) {
	r, err := m.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return mockRows{rows: r}, nil
}

func (m mockDB) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	return mockRow{row: m.pool.QueryRow(ctx, sql, args...)}
}

func (m mockDB) Begin(_ context.Context) (db.Tx, error) {
	return nil, errors.New("not implemented in tests")
}
