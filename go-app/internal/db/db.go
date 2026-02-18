// Package db contains PostgreSQL connection pool and query helpers.
package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is a global connection pool reference returned by Connect.
var Pool *pgxpool.Pool

// PoolAPI is an abstracted, mockable view of the pgx pool/tx used by code paths
// that require pgx-specific features like CopyFrom. It is initialized by Connect
// and can be overridden in tests.
var PoolAPI PoolIface

// DB is a minimal database interface to enable offline tests and pgxmock.
// Keep this surface area small; prefer repository-local helpers if you need more.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) error
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Begin(ctx context.Context) (Tx, error)
}

// Tx is a minimal transaction interface used by a few repos.
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) error
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// CopyFromTx is a transaction interface that supports pgx's CopyFrom in addition
// to the standard Tx methods. Used for high-throughput bulk inserts.
type CopyFromTx interface {
	Exec(ctx context.Context, sql string, args ...any) error
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, src pgx.CopyFromSource) (int64, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// PoolIface abstracts the subset of pgxpool.Pool we need, allowing pgxmock-based
// tests to inject a fake pool.
type PoolIface interface {
	Exec(ctx context.Context, sql string, args ...any) error
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Begin(ctx context.Context) (CopyFromTx, error)
}

// Row allows scanning a single row.
type Row interface {
	Scan(dest ...any) error
}

// Rows is a minimal row iterator abstraction for tests.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// defaultDB is the package-level DB used by helpers; set by Connect or tests.
var defaultDB DB

// SetDB allows tests to inject a fake DB implementation.
func SetDB(d DB) { defaultDB = d }

// ErrDBNotSet is returned when a DB helper is called before Connect or SetDB.
var ErrDBNotSet = errors.New("db not initialized")

// Exec runs a statement using the default DB.
func Exec(ctx context.Context, sql string, args ...any) error {
	if defaultDB == nil {
		return ErrDBNotSet
	}
	return defaultDB.Exec(ctx, sql, args...)
}

// Query runs a query returning multiple rows using the default DB.
func Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	if defaultDB == nil {
		return nil, ErrDBNotSet
	}
	return defaultDB.Query(ctx, sql, args...)
}

// QueryRow runs a query expecting a single row using the default DB.
func QueryRow(ctx context.Context, sql string, args ...any) Row {
	if defaultDB == nil {
		return &errRow{err: ErrDBNotSet}
	}
	return defaultDB.QueryRow(ctx, sql, args...)
}

// Begin starts a transaction using the default DB.
func Begin(ctx context.Context) (Tx, error) {
	if defaultDB == nil {
		return nil, ErrDBNotSet
	}
	return defaultDB.Begin(ctx)
}

// errRow implements Row and returns the wrapped error on Scan.
type errRow struct{ err error }

func (e *errRow) Scan(_ ...any) error { return e.err }

// poolDB adapts pgxpool.Pool to the DB interface.
type poolDB struct{ p *pgxpool.Pool }

func (w poolDB) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := w.p.Exec(ctx, sql, args...)
	return err
}

func (w poolDB) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	r, err := w.p.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rowsAdapter{r}, nil
}

func (w poolDB) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return rowAdapter{w.p.QueryRow(ctx, sql, args...)}
}

func (w poolDB) Begin(ctx context.Context) (Tx, error) {
	tx, err := w.p.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return txAdapter{tx}, nil
}

// poolAPIAdapter adapts pgxpool.Pool to PoolIface.
type poolAPIAdapter struct{ p *pgxpool.Pool }

func (a poolAPIAdapter) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := a.p.Exec(ctx, sql, args...)
	return err
}

func (a poolAPIAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	r, err := a.p.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rowsAdapter{r}, nil
}

func (a poolAPIAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return rowAdapter{a.p.QueryRow(ctx, sql, args...)}
}

func (a poolAPIAdapter) Begin(ctx context.Context) (CopyFromTx, error) {
	tx, err := a.p.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return txCopyAdapter{Tx: tx}, nil
}

type rowsAdapter struct{ pgx.Rows }

func (r rowsAdapter) Close() { r.Rows.Close() }

type rowAdapter struct{ pgx.Row }

// Scan proxies to the underlying row's Scan.
func (r rowAdapter) Scan(dest ...any) error { return r.Row.Scan(dest...) }

type txAdapter struct{ pgx.Tx }

func (t txAdapter) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := t.Tx.Exec(ctx, sql, args...)
	return err
}

func (t txAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	r, err := t.Tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rowsAdapter{r}, nil
}

func (t txAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return rowAdapter{t.Tx.QueryRow(ctx, sql, args...)}
}

func (t txAdapter) Commit(ctx context.Context) error   { return t.Tx.Commit(ctx) }
func (t txAdapter) Rollback(ctx context.Context) error { return t.Tx.Rollback(ctx) }

// txCopyAdapter adapts pgx.Tx to CopyFromTx.
type txCopyAdapter struct{ Tx pgx.Tx }

func (t txCopyAdapter) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := t.Tx.Exec(ctx, sql, args...)
	return err
}

func (t txCopyAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	r, err := t.Tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rowsAdapter{r}, nil
}

func (t txCopyAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return rowAdapter{t.Tx.QueryRow(ctx, sql, args...)}
}

func (t txCopyAdapter) CopyFrom(
	ctx context.Context,
	table pgx.Identifier,
	columns []string,
	src pgx.CopyFromSource,
) (int64, error) {
	return t.Tx.CopyFrom(ctx, table, columns, src)
}

func (t txCopyAdapter) Commit(ctx context.Context) error   { return t.Tx.Commit(ctx) }
func (t txCopyAdapter) Rollback(ctx context.Context) error { return t.Tx.Rollback(ctx) }

// BuildDSN composes a PostgreSQL DSN from individual parts. Pure helper for testing.
func BuildDSN(user, pass, host, port, database, ssl string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, database, ssl)
}

// Connect initializes a pgx connection pool using environment variables:
// POSTGRES_HOST, POSTGRES_PORT, POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_SSLMODE
func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")
	db := getenv("POSTGRES_DB", "cricket_data")
	user := getenv("POSTGRES_USER", "postgres")
	pass := getenv("POSTGRES_PASSWORD", "postgres")
	ssl := getenv("POSTGRES_SSLMODE", "disable")

	dsn := BuildDSN(user, pass, host, port, db, ssl)
	slog.Info("db.Connect: parsing config", slog.String("host", host), slog.String("db", db))
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		slog.Error("db.Connect: parse config failed", slog.Any("err", err))
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	// Use background context for pool lifecycle so it doesn't close if Connect's ctx is canceled/times out.
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		slog.Error("db.Connect: create pool failed", slog.Any("err", err))
		return nil, err
	}
	// Verify connectivity using the provided (potentially short-lived) context.
	if err := pool.Ping(ctx); err != nil {
		slog.Error("db.Connect: ping failed", slog.Any("err", err))
		pool.Close()
		return nil, err
	}
	slog.Info("db.Connect: connected successfully", slog.String("host", host), slog.String("db", db))
	Pool = pool
	defaultDB = poolDB{p: pool}
	PoolAPI = poolAPIAdapter{p: pool}
	return pool, nil
}

// SetPoolAPI allows tests to inject a mock pool implementation.
func SetPoolAPI(p PoolIface) { PoolAPI = p }

// Close closes the global connection pool if initialized. Safe to call multiple times.
// Call during graceful shutdown so in-flight connections drain and logs are flushed.
func Close() {
	if Pool == nil {
		return
	}
	Pool.Close()
	Pool = nil
	defaultDB = nil
	PoolAPI = nil
	slog.Info("db.Close: pool closed")
}

// RunInTx runs fn inside a transaction. Commits on success, rolls back on error or panic.
func RunInTx(ctx context.Context, fn func(ctx context.Context, tx CopyFromTx) error) error {
	if PoolAPI == nil {
		err := errors.New("db pool not initialized")
		slog.Error("db.RunInTx failed", slog.Any("err", err))
		return err
	}
	tx, err := PoolAPI.Begin(ctx)
	if err != nil {
		slog.Error("db.RunInTx Begin failed", slog.Any("err", err))
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, tx); err != nil {
		slog.Error("db.RunInTx fn failed", slog.Any("err", err))
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("db.RunInTx Commit failed", slog.Any("err", err))
		return err
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// RunMigrations executes .sql files in the given directory in lexical order.
// It creates a table schema_migrations(version text primary key, applied_at timestamptz) to track applied files.
// Implementation delegates to RunMigrationsFS for testability (behavior-preserving).
func RunMigrations(ctx context.Context, migrationsDir string) error {
	// Use an os-backed fs for the given directory and delegate to the FS-based runner.
	return RunMigrationsFS(ctx, os.DirFS(migrationsDir), ".")
}
