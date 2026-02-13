// Command weather-backfill backfills weather data for matches based on the queue.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

func main() {
	var (
		limit  = flag.Int("limit", 0, "maximum number of jobs to enqueue (0 = no limit)")
		dryRun = flag.Bool("dry-run", false, "show how many jobs would be enqueued without modifying the database")
	)
	flag.Parse()

	logger.SetupFromEnv()

	ctx := context.Background()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		os.Exit(1)
	}

	if *dryRun {
		slog.Info("dry-run: no changes made. Run without --dry-run to enqueue missing weather jobs.")
		return
	}

	added, err := db.EnqueueMissingWeatherJobs(ctx, *limit)
	if err != nil {
		slog.Error("enqueue missing jobs failed", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("enqueued weather jobs", slog.Int64("count", added), slog.Int("limit", *limit))
}
