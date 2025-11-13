// Command weather-import fetches weather for a match and upserts into DB.
// Thin wrapper: parse flags via internal CLI, wire dependencies, and delegate to
// internal runner/service. Behavior preserved (dry-run via --apply off).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	wrepo "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/db/weatherrepo"
	wprov "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/weather/dummy"
	wcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherimport"
	wcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/weatherimport"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	wsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherimport"
)

func main() { os.Exit(run()) }

func run() int {
	// Parse flags via internal CLI (unit-tested)
	fs := flag.NewFlagSet("weather-import", flag.ContinueOnError)
	opts, err := wcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		return 2
	}

	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}

	// Provider selection (default dummy). Additional providers can be added later.
	_ = strings.TrimSpace(opts.Provider) // reserved for future provider selection
	provider := wprov.New()
	repo := wrepo.New()
	svc := wsvc.NewService(provider, repo)
	runner := wcmd.NewRunner(svc)
	if runErr := runner.Run(ctx, opts); runErr != nil {
		slog.Error("weather-import failed", slog.Any("err", runErr))
		return 1
	}
	slog.Info("weather-import completed", slog.Int64("match", opts.MatchID))
	return 0
}
