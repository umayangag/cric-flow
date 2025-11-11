// Command etl-importer ingests curated CSVs into the database.
// Thin wrapper: parse flags via internal CLI, wire dependencies, and delegate to
// internal runner/service for execution. Behavior preserved (dry-run via --apply off).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	etlrepo "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/db/etlrepo"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/fsx/osfs"
	etlcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/etlimporter"
	etlcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/etlimporter"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	etlsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/etlimporter"
)

func main() {
	// parse flags using internal CLI (table-driven tests live in internal package)
	fs := flag.NewFlagSet("etl-importer", flag.ContinueOnError)
	opts, err := etlcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		os.Exit(2)
	}

	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}

	repo := etlrepo.New()
	service := etlsvc.NewService(osfs.New(), repo)
	runner := etlcmd.NewRunner(service)
	if runErr := runner.Run(ctx, opts); runErr != nil {
		slog.Error("etl-importer failed", slog.Any("err", runErr))
		os.Exit(1)
	}
	slog.Info("etl-importer completed")
}
