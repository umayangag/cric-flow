// Command cricsheet-importer imports Cricsheet JSON files into the database.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	cricsheetcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/cricsheetimporter"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
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

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}
	// Apply migrations to ensure schema is ready
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		return 1
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
		return 1
	}
	slog.Info("cricsheet-importer finished", slog.Int("files", n))
	return 0
}
