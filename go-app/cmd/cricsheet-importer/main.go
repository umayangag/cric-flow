// Command cricsheet-importer imports Cricsheet JSON files into the database.
// Uses pipeline.RunJob (shared with pipeline handler) for panic recovery and tracking.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	cricsheetcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/cricsheetimporter"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/pipeline"
)

func main() { os.Exit(run()) }

func run() (exitCode int) {
	fs := flag.NewFlagSet("cricsheet-importer", flag.ContinueOnError)
	copts, perr := cricsheetcli.ParseArgs(fs, os.Args[1:])
	if perr != nil {
		slog.Error("flag parsing failed", slog.Any("err", perr))
		return 2
	}

	logger.SetupFromEnv()
	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(baseCtx, copts.Timeout)
	defer cancel()

	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		return 1
	}

	opts := &cricsheet.Options{
		PlaceholdersWeather:  copts.PlaceholdersWeather,
		PlaceholdersFielding: copts.PlaceholdersFielding,
		WeatherEnqueue:       copts.WeatherEnqueue,
		FailFast:             copts.FailFast,
	}
	startMeta := map[string]any{"dir": copts.InDir}
	runErr := pipeline.RunJob(ctx, "cricsheet-import", startMeta, 0, func(jobCtx context.Context) (any, error) {
		n, err := cricsheet.ImportDir(jobCtx, copts.InDir, opts, copts.Concurrency)
		return map[string]any{"files": n, "dir": copts.InDir}, err
	})
	if runErr != nil {
		slog.Error("cricsheet import failed", slog.String("input_dir", copts.InDir), slog.Any("err", runErr))
		return 1
	}
	slog.Info("cricsheet-importer finished successfully", slog.String("dir", copts.InDir))
	return 0
}
