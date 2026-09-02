package opsstatus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// NewProductionDBProbe returns the default production implementation of DBProbe.
func NewProductionDBProbe() DBProbe { return productionDBProbe{} }

type productionDBProbe struct{}

func (productionDBProbe) Ping(ctx context.Context) error {
	if db.Pool == nil {
		return errors.New("db pool not initialized")
	}
	cfg := config.Load()
	sec := config.ServerDBProbeTimeoutSec(cfg)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(sec)*time.Second)
	defer cancel()
	return db.Pool.Ping(ctx)
}

func (productionDBProbe) Count(ctx context.Context, table string) (int64, error) {
	if db.Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	cfg := config.Load()
	sec := config.ServerDBProbeTimeoutSec(cfg)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(sec)*time.Second)
	defer cancel()

	var (
		n   int64
		sql string
	)
	switch strings.ToLower(strings.TrimSpace(table)) {
	case "players":
		sql = "SELECT COUNT(*) FROM player"
	case "matches":
		sql = "SELECT COUNT(*) FROM match"
	case "fielding_data":
		sql = "SELECT COUNT(*) FROM fielding_data"
	default:
		return 0, fmt.Errorf("unsupported table for count: %s", table)
	}
	row := db.Pool.QueryRow(ctx, sql)
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (productionDBProbe) CountFieldingByFormat(ctx context.Context, format string) (int64, error) {
	m, err := (productionDBProbe{}).CountFieldingByFormatGrouped(ctx)
	if err != nil {
		return 0, err
	}
	return m[format], nil
}

func (productionDBProbe) CountFieldingByFormatGrouped(ctx context.Context) (map[string]int64, error) {
	if db.Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	cfg := config.Load()
	sec := config.ServerDBProbeTimeoutSec(cfg)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(sec)*time.Second)
	defer cancel()
	rows, err := db.Pool.Query(ctx, `
        SELECT mf.code, COUNT(*)
        FROM fielding_data fd
        JOIN match m ON m.match_id = fd.match_id
        JOIN match_format mf ON mf.id = m.format_id
        GROUP BY mf.code
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var code string
		var n int64
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		out[code] = n
	}
	return out, rows.Err()
}

func (productionDBProbe) MigrationInfo(ctx context.Context) (int, int, string, error) {
	expected := countMigrationFiles()
	if db.Pool == nil {
		return 0, expected, "unknown", errors.New("db pool not initialized")
	}
	cfg := config.Load()
	sec := config.ServerDBProbeTimeoutSec(cfg)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(sec)*time.Second)
	defer cancel()
	var applied int
	if err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
		return 0, expected, "unknown", err
	}
	status := "ok"
	if applied < expected {
		status = "out_of_date"
	}
	return applied, expected, status, nil
}

func (productionDBProbe) LastMatchImportAt(ctx context.Context) (time.Time, error) {
	if db.Pool == nil {
		return time.Time{}, errors.New("db pool not initialized")
	}
	cfg := config.Load()
	sec := config.ServerDBProbeTimeoutSec(cfg)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(sec)*time.Second)
	defer cancel()
	var ts time.Time
	if err := db.Pool.QueryRow(ctx, "SELECT COALESCE(MAX(match_date), DATE '0001-01-01') FROM match").Scan(&ts); err != nil {
		return time.Time{}, err
	}
	if ts.IsZero() {
		return time.Time{}, errors.New("no match import date")
	}
	return ts.UTC(), nil
}

func (productionDBProbe) TableStats(ctx context.Context) ([]db.TableStat, error) {
	if db.Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	cfg := config.Load()
	sec := config.ServerDBProbeLongTimeoutSec(cfg)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(sec)*time.Second)
	defer cancel()
	return db.GetTableStats(ctx)
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
