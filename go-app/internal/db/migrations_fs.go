package db

import (
	"context"
	"fmt"
	"io/fs"
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
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".sql") {
			files = append(files, name)
		}
	}
	sort.Strings(files)

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

	for _, fname := range files {
		version := filepath.Base(fname)
		if applied[version] {
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
	}
	return nil
}
