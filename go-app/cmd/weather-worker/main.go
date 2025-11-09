// Command weather-worker dequeues weather jobs and fetches weather information.
package main

import (
	"context"
	"flag"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	weatherSvc "github.com/umayangag/cric-info-scrapers/go-app/internal/weather/service"
)

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(math.Min(float64(attempt), 300)) * time.Second // cap at 5m
	return d
}

func main() {
	var (
		batchSize   = flag.Int("batch-size", 5, "number of jobs to attempt per tick")
		intervalSec = flag.Int("interval", 5, "poll interval seconds")
		once        = flag.Bool("once", false, "process only one batch and exit")
		noop        = flag.Bool("noop", true, "dev-safe: do not call external APIs; requeue jobs with backoff")
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
	cfg := config.Load()

	slog.Info(
		"weather-worker started",
		slog.Int("batch", *batchSize),
		slog.Int("interval_sec", *intervalSec),
		slog.Bool("noop", *noop),
		slog.Int("rate_limit", cfg.Weather.RateLimitPerSec),
		slog.Int("max_attempts", cfg.Weather.MaxAttempts),
	)
	for {
		processed := 0
		for processed < *batchSize {
			job, err := db.DequeueNextWeatherJob(ctx, time.Now())
			if err != nil || job == nil {
				break // no jobs or error; sleep until next tick
			}
			res, perr := weatherSvc.ProcessJob(ctx, job, *noop)
			if perr != nil {
				next := time.Now().Add(backoff(job.Attempts + 1))
				if err2 := db.MarkWeatherJobFailed(ctx, job.ID, job.Attempts+1, perr.Error(), next); err2 != nil {
					slog.Warn("reschedule failed", slog.Int64("id", job.ID), slog.Any("err", err2))
				} else {
					slog.Info("job requeued", slog.Int64("id", job.ID), slog.Int64("match_id", job.MatchID), slog.Any("cause", perr), slog.String("next", next.Format(time.RFC3339)))
				}
			} else {
				if err := db.MarkWeatherJobDone(ctx, job.ID); err != nil {
					slog.Warn("mark job done failed", slog.Int64("id", job.ID), slog.Any("err", err))
				}
				if res != nil {
					slog.Info("job done", slog.Int64("id", job.ID), slog.Int64("match_id", job.MatchID), slog.String("venue", job.NormalizedVenue), slog.Int("upserts", res.Upserts), slog.Float64("lat", res.Lat), slog.Float64("lon", res.Lon), slog.String("tz", res.Timezone))
				}
			}
			processed++
		}
		if *once {
			break
		}
		time.Sleep(time.Duration(*intervalSec) * time.Second)
	}
	slog.Info("weather-worker stopped")
}
