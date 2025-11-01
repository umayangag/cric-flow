package main

import (
	"context"
	"flag"
	"log"
	"math"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	weatherSvc "github.com/umayangag/cric-info-scrapers/go-app/internal/weather/service"
)

func backoff(attempt int) time.Duration {
	if attempt < 1 { attempt = 1 }
	d := time.Duration(math.Min(float64(1<<uint(attempt)), 300)) * time.Second // cap at 5m
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

	ctx := context.Background()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}
	cfg := config.Load()

	log.Printf("weather-worker started (batch=%d interval=%ds noop=%v rate_limit=%d max_attempts=%d)", *batchSize, *intervalSec, *noop, cfg.Weather.RateLimitPerSec, cfg.Weather.MaxAttempts)
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
					log.Printf("warn: reschedule failed (id=%d): %v", job.ID, err2)
				} else {
					log.Printf("requeued job id=%d match_id=%d cause=%v next=%s", job.ID, job.MatchID, perr, next.Format(time.RFC3339))
				}
			} else {
				if err := db.MarkWeatherJobDone(ctx, job.ID); err != nil {
					log.Printf("warn: mark job done failed (id=%d): %v", job.ID, err)
				}
				if res != nil {
					log.Printf("done job id=%d match_id=%d venue=%s upserts=%d lat=%.4f lon=%.4f tz=%s", job.ID, job.MatchID, job.NormalizedVenue, res.Upserts, res.Lat, res.Lon, res.Timezone)
				}
			}
			processed++
		}
		if *once {
			break
		}
		time.Sleep(time.Duration(*intervalSec) * time.Second)
	}
	log.Printf("weather-worker stopped")
}
