// Command weather-worker fetches and upserts weather records for queued matches.
// Thin wrapper: parse via internal CLI, wire dependencies, delegate to internal runner.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strconv"
	"strings"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherworker"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/weatherworker"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherworker"
)

func main() { os.Exit(run()) }

func run() int {
	fs := flag.NewFlagSet("weather-worker", flag.ContinueOnError)
	opts, err := cli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parse failed", slog.Any("err", err))
		return 2
	}

	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}

	// Jobs source from env (comma separated match IDs), e.g., WEATHER_MATCH_IDS="1193505,1193506"
	parseIDs(os.Getenv("WEATHER_MATCH_IDS"))

	// Provider selection: currently only "dummy" wired; others can be added later.
	service := svc.NewService(nil, nil, nil)
	runner := cmd.NewRunner(service)
	if runErr := runner.Run(ctx, opts); runErr != nil {
		slog.Error("weather-worker failed", slog.Any("err", runErr))
		return 1
	}
	slog.Info("weather-worker completed")
	return 0
}

func parseIDs(csv string) []int64 {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.ParseInt(p, 10, 64); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}
