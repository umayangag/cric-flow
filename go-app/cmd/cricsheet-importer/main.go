// Command cricsheet-importer imports Cricsheet JSON files into the database.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	cricsheetcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/cricsheetimporter"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
)

func main() { os.Exit(run()) }

func run() int {
	// Parse flags via internal CLI to unify behavior and enable testing
	fs := flag.NewFlagSet("cricsheet-importer", flag.ContinueOnError)
	copts, perr := cricsheetcli.ParseArgs(fs, os.Args[1:])
	if perr != nil {
		slog.Error("flag parsing failed", slog.Any("err", perr))
		return 2
	}

	logger.SetupFromEnv()

	ctx := context.Background()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}
	// Apply migrations to ensure schema is ready
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		return 1
	}

	tracker, tErr := tracking.Start(ctx, "cricsheet-import", copts)
	if tErr != nil {
		slog.Warn("tracking start failed", slog.Any("err", tErr))
	}

	// Map CLI options to legacy cricsheet.Options to preserve behavior
	opts := &cricsheet.Options{
		PlaceholdersWeather:  copts.PlaceholdersWeather,
		PlaceholdersFielding: copts.PlaceholdersFielding,
		WeatherEnqueue:       copts.WeatherEnqueue,
	}
	n, err := cricsheet.ImportDir(ctx, copts.InDir, opts)
	if err != nil {
		slog.Error("cricsheet import failed", slog.Any("err", err))
		if tracker != nil {
			_ = tracker.Fail(ctx, err.Error())
		}
		return 1
	}
	if tracker != nil {
		_ = tracker.Complete(ctx, map[string]int{"files": n})
	}
	slog.Info("cricsheet-importer finished", slog.Int("files", n))
	return 0
}
