// Package db contains PostgreSQL connection pool and query helpers.
package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is a global connection pool reference returned by Connect.
var Pool *pgxpool.Pool

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
