// Command weather-import fetches weather for a match and upserts into DB.
// Option B implementation: this command is a thin wrapper around the
// weather worker service using a one-shot jobs.Source that yields a single
// match ID. Behavior preserved (dry-run via --apply off).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	wcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherimport"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/jobs"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	workersvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherworker"
)

func main() { os.Exit(run()) }

func run() int {
	// Parse flags via internal CLI (unit-tested)
	fs := flag.NewFlagSet("weather-import", flag.ContinueOnError)
	opts, err := wcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		return 2
	}

	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Hour)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}

	_ = strings.TrimSpace(opts.Provider) // reserved for future provider selection
	// TODO: add dependency
	// One-shot job source that yields exactly this match ID once.
	src := jobs.NewOneShotSource(opts.MatchID)
	svc := workersvc.NewService(src, nil, nil)
	if _, runErr := svc.Run(ctx, 1, opts.Apply); runErr != nil {
		slog.Error("weather-import failed", slog.Any("err", runErr))
		return 1
	}
	slog.Info("weather-import completed", slog.Int64("match", opts.MatchID))
	return 0
}
