// Command import-retired updates the is_retired flag in the player table from a CSV.
// See flags.go for CLI parsing and orch.go for orchestration.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

func main() {
	// Parse flags via pure function
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		os.Exit(2)
	}

	logger.SetupFromEnv()

	rows, err := parseCSV(opts.file)
	if err != nil {
		slog.Error("parse csv failed", slog.Any("err", err))
		os.Exit(1)
	}
	unique := map[string]struct{}{}
	var names []string
	for _, r := range rows {
		n := strings.TrimSpace(r.Name)
		if n == "" {
			continue
		}
		key := strings.ToLower(n)
		if _, ok := unique[key]; ok {
			continue
		}
		unique[key] = struct{}{}
        names = append(names, key)
	}

	if !opts.apply {
		fmt.Printf("[DRY-RUN] Would mark %d players as retired. others-zero=%v\n", len(names), opts.othersZero)
		for _, n := range names {
			fmt.Printf("  - %s\n", n)
		}
		if opts.othersZero {
			fmt.Println("[DRY-RUN] Would set is_retired=0 for all other players not listed")
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}
	// Optional: run migrations to ensure schema
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "/migrations"
	}
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		os.Exit(1)
	}

	changed, zeroed, err := applyRetired(ctx, names, opts.othersZero)
	if err != nil {
		slog.Error("apply failed", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("Applied", slog.Int64("marked_retired", changed), slog.Int64("zeroed_others", zeroed))
}
