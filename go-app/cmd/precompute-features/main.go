// Command precompute-features replays matches chronologically and persists
// leakage-free, date-indexed (as-of) feature snapshots per player.
// Uses pipeline.RunJob (shared with pipeline handler) for panic recovery and tracking.
// In replay mode, consider using precompute-all which calls precompute.Run.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pfcli "github.com/umayangag/cric-flow/go-app/internal/cli/precomputefeatures"
	pfcmd "github.com/umayangag/cric-flow/go-app/internal/commands/precomputefeatures"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
)

func main() { os.Exit(run()) }

func run() int {
	fs := flag.NewFlagSet("precompute-features", flag.ContinueOnError)
	opts, err := pfcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		return 2
	}

	logger.SetupFromEnv()

	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(baseCtx, opts.Timeout)
	defer cancel()

	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}

	if err := db.RunMigrations(ctx, opts.MigrationsDir); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		return 1
	}

	formatID, err := db.GetMatchFormatIDByCode(ctx, opts.Format)
	if err != nil {
		slog.Error("resolve format failed", slog.String("format", opts.Format), slog.Any("err", err))
		return 1
	}

	cfg := config.Load()
	windowN := 0
	if cfg != nil && cfg.Features.HistoryWindowMatches > 0 {
		windowN = cfg.Features.HistoryWindowMatches
	}

	startMeta := map[string]any{"format": opts.Format, "replay": opts.Replay}
	runErr := pipeline.RunJob(ctx, "precompute-features", startMeta, 0, func(jobCtx context.Context) (any, error) {
		runner := pfcmd.NewRunner()
		if opts.Replay {
			err := runner.RunReplay(jobCtx, opts.Format, formatID, opts.EWMAlpha, opts.LastN, windowN)
			return map[string]any{"type": "replay"}, err
		}
		var asOf time.Time
		if strings.TrimSpace(opts.AsOf) == "" {
			asOf = time.Now().UTC()
			slog.Info("no -as-of provided; defaulting to today (UTC)", slog.String("as_of", asOf.Format("2006-01-02")))
		} else {
			var parseErr error
			asOf, parseErr = time.Parse("2006-01-02", opts.AsOf)
			if parseErr != nil {
				return nil, parseErr
			}
		}
		err := runner.RunPointInTime(jobCtx, opts.Format, formatID, asOf, opts.EWMAlpha, opts.LastN, windowN)
		return map[string]any{"type": "as-of", "as_of": asOf.Format("2006-01-02")}, err
	})
	if runErr != nil {
		slog.Error("precompute-features failed", slog.Any("err", runErr))
		return 1
	}
	return 0
}
