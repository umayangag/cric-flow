package server

import (
    "context"
    "errors"
    "log/slog"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// DBProbe defines the minimal DB checks needed for /ops/status.
type DBProbe interface {
    Ping(ctx context.Context) error
    Count(ctx context.Context, table string) (int64, error)
    // MigrationInfo returns (currentApplied, expectedTotal, status)
    // status: "ok" | "unknown" | "out_of_date"
    MigrationInfo(ctx context.Context) (int, int, string, error)
}

// newProductionDBProbe returns the default production implementation.
func newProductionDBProbe() DBProbe { return productionDBProbe{} }

type productionDBProbe struct{}

func (productionDBProbe) Ping(ctx context.Context) error {
    if db.Pool == nil {
        return errors.New("db pool not initialized")
    }
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    return db.Pool.Ping(ctx)
}

func (productionDBProbe) Count(ctx context.Context, table string) (int64, error) {
    if db.Pool == nil {
        return 0, errors.New("db pool not initialized")
    }
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    var n int64
    row := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table)
    if err := row.Scan(&n); err != nil {
        return 0, err
    }
    return n, nil
}

func (productionDBProbe) MigrationInfo(ctx context.Context) (int, int, string, error) {
    expected := countMigrationFiles()
    if db.Pool == nil {
        return 0, expected, "unknown", errors.New("db pool not initialized")
    }
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    var applied int
    // Try to count rows in schema_migrations; if missing, mark unknown.
    if err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
        return 0, expected, "unknown", err
    }
    status := "ok"
    if applied < expected {
        status = "out_of_date"
    }
    return applied, expected, status, nil
}

func countMigrationFiles() int {
    dir := os.Getenv("GO_APP_MIGRATIONS_DIR")
    if strings.TrimSpace(dir) == "" {
        dir = "./migrations"
    }
    entries, err := os.ReadDir(dir)
    if err != nil {
        slog.Warn("ops-status: could not read migrations dir", slog.String("dir", dir), slog.Any("err", err))
        return 0
    }
    count := 0
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        name := e.Name()
        if strings.EqualFold(filepath.Ext(name), ".sql") {
            count++
        }
    }
    return count
}
