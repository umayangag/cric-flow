package db_test

import (
	"context"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// mockTxAPI adapts pgxmock's pool to db.CopyFromTx, so a repository function that writes
// on the caller's transaction can be tested for the statements it issues without a
// database. Shared by every *_test.go in this package that needs a transaction.
type mockTxAPI struct{ p pgxmock.PgxPoolIface }

func (t mockTxAPI) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := t.p.Exec(ctx, sql, args...)
	return err
}

func (t mockTxAPI) Query(ctx context.Context, sql string, args ...any) (db.Rows, error) {
	r, err := t.p.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rowsWrap{r}, nil
}

func (t mockTxAPI) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	return rowWrap{t.p.QueryRow(ctx, sql, args...)}
}

func (t mockTxAPI) CopyFrom(
	ctx context.Context,
	table pgx.Identifier,
	columns []string,
	src pgx.CopyFromSource,
) (int64, error) {
	return t.p.CopyFrom(ctx, table, columns, src)
}
func (t mockTxAPI) Commit(ctx context.Context) error   { return t.p.Commit(ctx) }
func (t mockTxAPI) Rollback(ctx context.Context) error { return t.p.Rollback(ctx) }

type rowsWrap struct{ pgx.Rows }

func (r rowsWrap) Close() { r.Rows.Close() }

type rowWrap struct{ pgx.Row }

func (r rowWrap) Scan(dest ...any) error { return r.Row.Scan(dest...) }
