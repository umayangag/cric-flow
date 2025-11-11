// Package db contains PostgreSQL connection pool and query helpers.
package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	cfgpkg "github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// Pool is a global connection pool reference returned by Connect.
var Pool *pgxpool.Pool

// DB is a minimal database interface to enable offline tests.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) error
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
}

// Rows is a minimal row iterator abstraction for tests.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
}

// defaultDB is the package-level DB used by helpers; set by Connect or tests.
var defaultDB DB

// SetDB allows tests to inject a fake DB implementation.
func SetDB(d DB) { defaultDB = d }

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

type rowsAdapter struct{ pgx.Rows }

func (r rowsAdapter) Close() { r.Rows.Close() }

// BuildDSN composes a PostgreSQL DSN from individual parts. Pure helper for testing.
func BuildDSN(user, pass, host, port, database, ssl string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, database, ssl)
}

// Connect initializes a pgx connection pool.
// Precedence: env vars > config.json > built-in defaults.
// Env vars: POSTGRES_HOST, POSTGRES_PORT, POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_SSLMODE
func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	cfgJSON := cfgpkg.Load()

	// Defaults from config.json if present; else built-ins
	defHost := "localhost"
	defPort := "5432"
	defDB := "cricket_data"
	defUser := "postgres"
	defPass := "postgres"
	defSSL := "disable"
	if cfgJSON != nil {
		if v := cfgJSON.Database.Host; v != "" {
			defHost = v
		}
		if v := cfgJSON.Database.Port; v != "" {
			defPort = v
		}
		if v := cfgJSON.Database.Name; v != "" {
			defDB = v
		}
		if v := cfgJSON.Database.User; v != "" {
			defUser = v
		}
		if v := cfgJSON.Database.Password; v != "" {
			defPass = v
		}
		if v := cfgJSON.Database.SSLMode; v != "" {
			defSSL = v
		}
	}

	host := getenv("POSTGRES_HOST", defHost)
	port := getenv("POSTGRES_PORT", defPort)
	db := getenv("POSTGRES_DB", defDB)
	user := getenv("POSTGRES_USER", defUser)
	pass := getenv("POSTGRES_PASSWORD", defPass)
	ssl := getenv("POSTGRES_SSLMODE", defSSL)

	dsn := BuildDSN(user, pass, host, port, db, ssl)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	Pool = pool
	defaultDB = poolDB{p: pool}
	return pool, nil
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
