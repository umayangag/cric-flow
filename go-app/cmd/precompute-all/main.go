// Command precompute-all runs both as-of/replay feature precomputation and
// sequential feature calculations in a single invocation.
// In replay mode, reuses precompute.Run (same logic as pipeline handler).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	pacli "github.com/umayangag/cric-flow/go-app/internal/services/precomputeall"
	pfsvc "github.com/umayangag/cric-flow/go-app/internal/services/precomputefeatures"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/precompute"
	"github.com/umayangag/cric-flow/go-app/internal/seqcalc"
)

func main() { os.Exit(run()) }

func run() int {
	// Parse flags via internal CLI (unified)
	fs := flag.NewFlagSet("precompute-all", flag.ContinueOnError)
	opts, err := pacli.ParseArgs(fs, os.Args[1:])
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

	// -all-formats: run all canonical formats in parallel (same as pipeline API)
	if opts.AllFormats {
		precomputeOpts := &precompute.RunOpts{Alpha: opts.EWMAlpha, LastN: opts.LastN}
		meta := map[string]any{"all_formats": true, "replay": true}
		runErr := pipeline.RunJob(
			ctx,
			"precompute-features",
			meta,
			opts.Timeout,
			func(jobCtx context.Context) (any, error) {
				return meta, precompute.Run(jobCtx, "", formats.CanonicalCodes(), precomputeOpts)
			},
		)
		if runErr != nil {
			slog.Error("precompute all-formats failed", slog.Any("err", runErr))
			return 1
		}
		return 0
	}

	// Resolve format_id
	formatID, err := db.GetMatchFormatIDByCode(ctx, opts.Format)
	if err != nil {
		slog.Error("resolve format failed", slog.String("format", opts.Format), slog.Any("err", err))
		return 1
	}

	// Read optional history window from config (same as precompute-features)
	cfg := config.Load()
	windowN := 0
	if cfg != nil && cfg.Features.HistoryWindowMatches > 0 {
		windowN = cfg.Features.HistoryWindowMatches
	}

	// Determine as-of mode and parse as-of date exactly once
	isAsOfMode := !opts.Replay
	var asOf time.Time
	haveAsOf := false
	if isAsOfMode {
		if strings.TrimSpace(opts.AsOf) == "" {
			asOf = time.Now().UTC()
			haveAsOf = true
			slog.Info("no -as-of provided; defaulting to today (UTC)", slog.String("as_of", asOf.Format("2006-01-02")))
		} else {
			var parseErr error
			asOf, parseErr = time.Parse("2006-01-02", opts.AsOf)
			if parseErr != nil {
				slog.Error("parse -as-of failed", slog.Any("err", parseErr))
				return 1
			}
			haveAsOf = true
		}
	} else {
		// In replay mode, -as-of is ignored for precompute-features but can still be used by seqcalc
		if s := strings.TrimSpace(opts.AsOf); s != "" {
			if t, perr := time.Parse("2006-01-02", s); perr == nil {
				asOf = t
				haveAsOf = true
			} else {
				slog.Warn("invalid -as-of provided; ignoring for seq stage", slog.String("as_of", s), slog.Any("err", perr))
			}
		}
	}

	// Run base precompute features first. Replay uses precompute.Run (shared with pipeline handler).
	meta := map[string]any{"format": opts.Format, "replay": opts.Replay}
	runErr := pipeline.RunJob(ctx, "precompute-features", meta, 0, func(jobCtx context.Context) (any, error) {
		if opts.Replay {
			precomputeOpts := &precompute.RunOpts{Alpha: opts.EWMAlpha, LastN: opts.LastN}
			return meta, precompute.Run(jobCtx, "", []string{opts.Format}, precomputeOpts)
		}
		// Single-date mode (as-of): not supported by precompute.Run, use runner directly
		runner := pfsvc.NewRunner()
		if err := runner.RunPointInTime(jobCtx, opts.Format, formatID, asOf, opts.EWMAlpha, opts.LastN, windowN, 0); err != nil {
			return nil, err
		}
		return meta, nil
	})
	if runErr != nil {
		slog.Error("precompute failed", slog.Any("err", runErr))
		return 1
	}

	// Replay mode: precompute.Run already runs seqcalc via RunReplay. As-of mode: run seqcalc here.
	if opts.Replay {
		return 0
	}

	params := seqcalc.Params{FormatCode: opts.Format}
	if haveAsOf {
		params.AsOf = asOf
	}
	reg := seqcalc.NewDefaultRegistry()
	calcs, err := reg.ResolveTargets(opts.SeqTargets)
	if err != nil {
		slog.Error("seq targets resolve failed", slog.Any("err", err))
		return 1
	}
	if opts.SeqDryRun {
		if err := seqcalc.DryRun(os.Stdout, calcs, params); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if err := seqcalc.Run(ctx, calcs, params, false); err != nil {
		slog.Error("sequence features run failed", slog.Any("err", err))
		return 1
	}
	return 0
}
