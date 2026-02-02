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

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// DBProbe defines the minimal DB checks needed for /ops/status.
type DBProbe interface {
	Ping(ctx context.Context) error
	Count(ctx context.Context, table string) (int64, error)
	// MigrationInfo returns (currentApplied, expectedTotal, status)
	// status: "ok" | "unknown" | "out_of_date"
	MigrationInfo(ctx context.Context) (int, int, string, error)
	// LastMatchImportAt returns the latest available import timestamp/date for match data
	// based on the `match_details` table. If not available, returns zero time with error.
	LastMatchImportAt(ctx context.Context) (time.Time, error)
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
	// Our actual schema names differ (player/match_details). Map friendly names
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
		// Distinct matches are identified by match_details.match_id
		sql = "SELECT COUNT(DISTINCT match_id) FROM match_details"
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
	// `date` column is DATE; cast to timestamptz at midnight UTC for display
	var ts time.Time
	// Prefer a real timestamp column if exists; fall back to date
	// Try updated_at on match_details (if present in later migrations); ignore error and fall back
	if err := db.Pool.QueryRow(ctx, "SELECT COALESCE(MAX(updated_at), TO_TIMESTAMP(0)) FROM match_details").Scan(&ts); err == nil &&
		!ts.IsZero() {
		return ts.UTC(), nil
	}
	// Fallback: max(date)
	if err := db.Pool.QueryRow(ctx, "SELECT COALESCE(MAX(date), DATE '0001-01-01') FROM match_details").Scan(&ts); err != nil {
		return time.Time{}, err
	}
	if ts.IsZero() {
		return time.Time{}, errors.New("no match import date")
	}
	return ts.UTC(), nil
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
