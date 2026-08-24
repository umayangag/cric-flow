package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
)

// RunMigrationsFS executes .sql files available via the provided fs.FS under the given dir.
// Behavior mirrors RunMigrations but uses an abstract filesystem for testability.
func RunMigrationsFS(ctx context.Context, fsys fs.FS, dir string) error {
	// Ensure a DB adapter is available
	if defaultDB == nil {
		if _, err := Connect(ctx); err != nil {
			return err
		}
	}
	// ensure table exists
	if err := defaultDB.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	var files []string
	skippedDown := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		// Rollback scripts are never applied forward. Without this they would be treated
		// as ordinary migrations and, because "down" sorts before "up", would run before
		// the migration they undo.
		if isDownMigration(name) {
			skippedDown++
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)
	slog.Info(
		"migrations: scanned directory",
		slog.String("dir", dir),
		slog.Int("files_total", len(files)),
		slog.Int("down_files_skipped", skippedDown),
	)

	// get applied versions
	applied := map[string]bool{}
	rows, err := defaultDB.Query(ctx, `SELECT version FROM schema_migrations`)
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

	appliedCount := 0
	skippedCount := 0
	for _, fname := range files {
		version := filepath.Base(fname)
		if applied[version] {
			skippedCount++
			continue
		}
		b, err := fs.ReadFile(fsys, filepath.Join(dir, fname))
		if err != nil {
			return err
		}
		sql := string(b)
		if err := defaultDB.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migration %s failed: %w", version, err)
		}
		if err := defaultDB.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, version); err != nil {
			return err
		}
		appliedCount++
		slog.Info("migrations: applied", slog.String("version", version))
	}
	slog.Info(
		"migrations: done",
		slog.Int("applied", appliedCount),
		slog.Int("skipped", skippedCount),
		slog.Int("seen", len(files)),
	)
	return nil
}

// isDownMigration reports whether a migration filename is a rollback script.
// Rollback scripts are named "<version>.down.sql" and are only ever run by hand.
func isDownMigration(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".down.sql")
}
