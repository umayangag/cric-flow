// Command backfill-fielding rebuilds fielding_event aggregates into fielding_data.
// Thin wrapper: parse flags via internal CLI, wire dependencies, and delegate to
// internal runner/service for execution. Behavior preserved (dry-run via --apply off).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	bfrepo "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/db/fieldingrepo"
	bfcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/backfillfielding"
	bfcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/backfillfielding"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	bfsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/fielding"
)

func main() { os.Exit(run()) }

func run() int {
	// parse flags using internal CLI (table-driven tests live in internal package)
	fs := flag.NewFlagSet("backfill-fielding", flag.ContinueOnError)
	opts, err := bfcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		return 2
	}

	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}

	repo := bfrepo.New()
	svc := bfsvc.NewService(repo)
	runner := bfcmd.NewRunner(svc)
	if runErr := runner.Run(ctx, opts); runErr != nil {
		slog.Error("backfill-fielding failed", slog.Any("err", runErr))
		return 1
	}
	slog.Info("backfill-fielding completed")
	return 0
}
