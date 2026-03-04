// Package connection provides PostgreSQL pool creation and lifecycle.
// The db package wires this into global state (Pool, defaultDB, PoolAPI) for backward compatibility.
package connection

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BuildDSN composes a PostgreSQL DSN from individual parts. Pure helper for testing.
func BuildDSN(user, pass, host, port, database, ssl string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, database, ssl)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Connect creates a new pgx connection pool using environment variables:
// POSTGRES_HOST, POSTGRES_PORT, POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_SSLMODE.
// The returned pool is not stored globally; the caller (db package) wires it into package-level state.
func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")
	database := getenv("POSTGRES_DB", "cricket_data")
	user := getenv("POSTGRES_USER", "postgres")
	pass := getenv("POSTGRES_PASSWORD", "postgres")
	ssl := getenv("POSTGRES_SSLMODE", "disable")

	dsn := BuildDSN(user, pass, host, port, database, ssl)
	slog.Info("db.Connect: parsing config", slog.String("host", host), slog.String("db", database))
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		slog.Error("db.Connect: parse config failed", slog.Any("err", err))
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		slog.Error("db.Connect: create pool failed", slog.Any("err", err))
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		slog.Error("db.Connect: ping failed", slog.Any("err", err))
		pool.Close()
		return nil, err
	}
	slog.Info("db.Connect: connected successfully", slog.String("host", host), slog.String("db", database))
	return pool, nil
}

// Close closes the given pool. Safe to call with nil. Call during graceful shutdown.
func Close(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	pool.Close()
	slog.Info("db.Close: pool closed")
}
