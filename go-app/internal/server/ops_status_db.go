package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// DBProbe defines the minimal DB checks needed for /ops/status.
type DBProbe interface {
	Ping(ctx context.Context) error
	Count(ctx context.Context, table string) (int64, error)
	// MigrationInfo returns (currentApplied, expectedTotal, status)
	// status: "ok" | "unknown" | "out_of_date"
	MigrationInfo(ctx context.Context) (int, int, string, error)
	// LastMatchImportAt returns the latest available import timestamp/date for match data
	// based on the `match` table. If not available, returns zero time with error.
	LastMatchImportAt(ctx context.Context) (time.Time, error)
	// TableStats returns row counts and last record info for all tables.
	TableStats(ctx context.Context) ([]db.TableStat, error)
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
	// The Ops dashboard asks for logical entity counts: "players", "matches", "innings".
	// Our actual schema names differ (player/match). Map friendly names
	// to the correct SQL so the dashboard reflects real data after imports.
	if db.Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var (
		n   int64
		sql string
	)
	switch strings.ToLower(strings.TrimSpace(table)) {
	case "players":
		// Schema table is singular: player
		sql = "SELECT COUNT(*) FROM player"
	case "matches":
		// Distinct matches are identified by match.match_id
		sql = "SELECT COUNT(*) FROM match"
	case "fielding_data":
		sql = "SELECT COUNT(*) FROM fielding_data"
	case "weather_data":
		sql = "SELECT COUNT(*) FROM weather_data"
	default:
		// Reject unknown table names to avoid SQL injection risks.
		return 0, fmt.Errorf("unsupported table for count: %s", table)
	}
	row := db.Pool.QueryRow(ctx, sql)
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

func (productionDBProbe) LastMatchImportAt(ctx context.Context) (time.Time, error) {
	if db.Pool == nil {
		return time.Time{}, errors.New("db pool not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// Use match_date for latest available match record date.
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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.GetTableStats(ctx)
}

// buildDBSection assembles the DB section map for /ops/status using the provided probe.
// It mirrors the previous inline logic but keeps concerns localized and testable.
func buildDBSection(ctx context.Context, probe DBProbe) map[string]any {
	out := map[string]any{"connected": false}
	if probe == nil {
		out["migration"] = map[string]any{"status": "unknown"}
		return out
	}
	// Ping with a short timeout at the callsite if needed; here we assume ctx has one.
	if err := probe.Ping(ctx); err == nil {
		out["connected"] = true
		counts := map[string]int64{}
		if n, err := probe.Count(ctx, "players"); err == nil {
			counts["players"] = n
		}
		if n, err := probe.Count(ctx, "matches"); err == nil {
			counts["matches"] = n
		}
		if len(counts) > 0 {
			out["counts"] = counts
		}
		if cur, exp, status, err := probe.MigrationInfo(ctx); err == nil {
			out["migration"] = map[string]any{"status": status, "current": cur, "expected": exp}
		} else {
			out["migration"] = map[string]any{"status": "unknown"}
		}
		if last, err := probe.LastMatchImportAt(ctx); err == nil && !last.IsZero() {
			out["last_match_import_at"] = last.UTC().Format(time.RFC3339)
		}
		if stats, err := probe.TableStats(ctx); err == nil {
			out["table_stats"] = stats
		}
		return out
	}
	// Disconnected path: try to surface expected migrations if known
	if _, exp, status, _ := probe.MigrationInfo(ctx); exp > 0 {
		out["migration"] = map[string]any{"status": status, "expected": exp}
	} else {
		out["migration"] = map[string]any{"status": "unknown"}
	}
	return out
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
