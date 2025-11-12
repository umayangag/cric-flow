// Command import-retired updates the is_retired flag in the player table from a CSV.
// Thin entrypoint: parse flags via internal CLI, parse CSV via internal/csvx,
// and delegate dry-run/apply to internal command runner.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/importretired"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/importretired"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/csvx"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

func main() {
	// Parse flags via internal CLI (testable)
	fs := flag.NewFlagSet("import-retired", flag.ContinueOnError)
	opts, err := cli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		os.Exit(2)
	}

	logger.SetupFromEnv()

	rows, err := csvx.ParseRetiredCSV(os.DirFS("."), opts.File)
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
		names = append(names, n)
	}

	runner := cmd.NewRunner(cmd.NewDB())
	if !opts.Apply {
		if err := runner.DryRun(names, opts.OthersZero, os.Stdout); err != nil {
			slog.Error("dry-run failed", slog.Any("err", err))
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
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

	changed, zeroed, err := runner.Apply(ctx, names, opts.OthersZero)
	if err != nil {
		slog.Error("apply failed", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("Applied", slog.Int64("marked_retired", changed), slog.Int64("zeroed_others", zeroed))
}
