// Command import-keepers updates the is_wicket_keeper flag for players from a CSV.
// See flags.go for CLI parsing and orch.go for orchestration.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/importkeepers"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/importkeepers"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/csvx"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

func main() {
	// Parse flags via internal CLI
	fs := flag.NewFlagSet("import-keepers", flag.ContinueOnError)
	opts, err := cli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		os.Exit(2)
	}

	logger.SetupFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}

	rows, err := csvx.ParseKeepersCSV(os.DirFS("."), opts.File)
	if err != nil {
		slog.Error("parse CSV failed", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("parsed keeper rows", slog.Int("rows", len(rows)), slog.String("file", opts.File))

	// Build target map
	targets := make(map[string]int, len(rows))
	for _, r := range rows {
		targets[strings.ToLower(r.Name)] = r.Value
	}

	runner := cmd.NewRunner(cmd.NewDB())
	if !opts.Apply {
		if err := runner.Preview(ctx, targets, opts.OthersZero); err != nil {
			slog.Error("dry-run failed", slog.Any("err", err))
			os.Exit(1)
		}
		fmt.Println("dry-run complete. Re-run with --apply to persist changes.")
		return
	}
	if err := runner.Apply(ctx, targets, opts.OthersZero); err != nil {
		slog.Error("apply failed", slog.Any("err", err))
		os.Exit(1)
	}
	fmt.Println("apply complete.")
}
