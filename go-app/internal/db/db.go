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

// Connect initializes a pgx connection pool using environment variables:
// POSTGRES_HOST, POSTGRES_PORT, POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_SSLMODE
func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")
	db := getenv("POSTGRES_DB", "cricket_data")
	user := getenv("POSTGRES_USER", "postgres")
	pass := getenv("POSTGRES_PASSWORD", "postgres")
	ssl := getenv("POSTGRES_SSLMODE", "disable")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, db, ssl)
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
func RunMigrations(ctx context.Context, migrationsDir string) error {
	if Pool == nil {
		if _, err := Connect(ctx); err != nil {
			return err
		}
	}
	// ensure table
	_, err := Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".sql") {
			files = append(files, filepath.Join(migrationsDir, name))
		}
	}
	sort.Strings(files)

	// get applied versions
	applied := map[string]bool{}
	rows, err := Pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}

	for _, f := range files {
		version := filepath.Base(f)
		if applied[version] {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		sql := string(b)
		// execute as one Exec (allow multiple statements)
		// pgx doesn't support multi-statement via Batch directly; use Exec instead.
		// We'll just run Exec with the whole content.
		if _, err := Pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migration %s failed: %w", version, err)
		}
		if _, err := Pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, version); err != nil {
			return err
		}
	}
	return nil
}
