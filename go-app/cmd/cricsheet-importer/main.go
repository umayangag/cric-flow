// Command cricsheet-importer imports Cricsheet JSON files into the database.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

func main() {
	// Resolve default input directory from env or config fallback
	defDataDir := os.Getenv("GO_APP_INPUT_DIR")
	if defDataDir == "" {
		defDataDir = config.DefaultCricsheetDir()
	}

	var (
		dataDir = flag.String(
			"dir",
			defDataDir,
			"Directory containing Cricsheet .json files (default from GO_APP_INPUT_DIR or ../data/go-app)",
		)
		phWeather = flag.Bool("placeholders-weather", false, "Insert placeholder weather rows per match")
		phField   = flag.Bool("placeholders-fielding", false, "Insert zeroed fielding rows for all players seen")
		wEnqueue  = flag.Bool("weather-enqueue", true, "Enqueue async weather jobs per match (non-blocking)")
	)
	flag.Parse()

	logger.SetupFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}
	// Apply migrations to ensure schema is ready
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		os.Exit(1)
	}

	opts := &cricsheet.Options{
		PlaceholdersWeather:  *phWeather,
		PlaceholdersFielding: *phField,
		WeatherEnqueue:       *wEnqueue,
	}
	n, err := cricsheet.ImportDir(ctx, *dataDir, opts)
	if err != nil {
		slog.Error("cricsheet import failed", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("cricsheet-importer finished", slog.Int("files", n))
}
