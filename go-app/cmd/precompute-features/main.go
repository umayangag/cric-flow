// Command precompute-features replays matches chronologically and persists
// leakage-free, date-indexed (as-of) feature snapshots per player.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	pfcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/precomputefeatures"
	pfcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/precomputefeatures"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
)

func main() { os.Exit(run()) }

func run() int {
	// Parse flags via internal CLI
	fs := flag.NewFlagSet("precompute-features", flag.ContinueOnError)
	opts, err := pfcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		return 2
	}

	logger.SetupFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	// Ensure DB connection
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}

	// Apply migrations (flag may override env via CLI defaults)
	migDir := opts.MigrationsDir
	if err := db.RunMigrations(ctx, migDir); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		return 1
	}

	tracker, tErr := tracking.Start(ctx, "precompute-features", opts)
	if tErr != nil {
		slog.Warn("tracking start failed", slog.Any("err", tErr))
	}

	formatID, err := db.GetMatchFormatIDByCode(ctx, opts.Format)
	if err != nil {
		slog.Error("resolve format failed", slog.String("format", opts.Format), slog.Any("err", err))
		tracker.TryFail(ctx, err.Error())
		return 1
	}

	// Read optional history window from config
	cfg := config.Load()
	windowN := 0
	if cfg != nil && cfg.Features.HistoryWindowMatches > 0 {
		windowN = cfg.Features.HistoryWindowMatches
	}

	runner := pfcmd.NewRunner()
	if opts.Replay {
		if err := runner.RunReplay(ctx, opts.Format, formatID, opts.EWMAlpha, opts.LastN, windowN); err != nil {
			slog.Error("replay failed", slog.Any("err", err))
			tracker.TryFail(ctx, err.Error())
			return 1
		}
		tracker.TryComplete(ctx, map[string]string{"type": "replay"})
		return 0
	}

	// Single-date mode (as-of)
	var asOf time.Time
	if strings.TrimSpace(opts.AsOf) == "" {
		// Default to today's date in UTC when -as-of is not provided
		asOf = time.Now().UTC()
		slog.Info("no -as-of provided; defaulting to today (UTC)", slog.String("as_of", asOf.Format("2006-01-02")))
	} else {
		var parseErr error
		asOf, parseErr = time.Parse("2006-01-02", opts.AsOf)
		if parseErr != nil {
			slog.Error("parse -as-of failed", slog.Any("err", parseErr))
			tracker.TryFail(ctx, parseErr.Error())
			return 1
		}
	}
	if err := runner.RunPointInTime(ctx, opts.Format, formatID, asOf, opts.EWMAlpha, opts.LastN, windowN); err != nil {
		slog.Error("as-of run failed", slog.Any("err", err))
		tracker.TryFail(ctx, err.Error())
		return 1
	}
	tracker.TryComplete(ctx, map[string]string{"type": "as-of", "as_of": asOf.Format("2006-01-02")})
	return 0
}
