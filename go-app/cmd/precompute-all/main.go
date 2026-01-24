// Command precompute-all runs both as-of/replay feature precomputation and
// sequential feature calculations in a single invocation.
package main

import (
    "context"
    "flag"
    "fmt"
    "log/slog"
    "os"
    "strings"
    "time"

    pacli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/precomputeall"
    pfcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/precomputefeatures"
    "github.com/umayangag/cric-info-scrapers/go-app/internal/config"
    "github.com/umayangag/cric-info-scrapers/go-app/internal/db"
    "github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
    "github.com/umayangag/cric-info-scrapers/go-app/internal/seqcalc"
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

    // Apply migrations
    migDir := opts.MigrationsDir
    if env := os.Getenv("MIGRATIONS_DIR"); env != "" {
        migDir = env
    }
    if err := db.RunMigrations(ctx, migDir); err != nil {
        slog.Error("migrations failed", slog.Any("err", err))
        return 1
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

    // Run base precompute features first
    runner := pfcmd.NewRunner()
    if opts.Replay {
        if err := runner.RunReplay(ctx, opts.Format, formatID, opts.EWMAlpha, opts.LastN, windowN); err != nil {
            slog.Error("replay precompute failed", slog.Any("err", err))
            return 1
        }
    } else {
        // Single-date mode (as-of)
        var asOf time.Time
        if strings.TrimSpace(opts.AsOf) == "" {
            asOf = time.Now().UTC()
            slog.Info("no -as-of provided; defaulting to today (UTC)", slog.String("as_of", asOf.Format("2006-01-02")))
        } else {
            var parseErr error
            asOf, parseErr = time.Parse("2006-01-02", opts.AsOf)
            if parseErr != nil {
                slog.Error("parse -as-of failed", slog.Any("err", parseErr))
                return 1
            }
        }
        if err := runner.RunPointInTime(ctx, opts.Format, formatID, asOf, opts.EWMAlpha, opts.LastN, windowN); err != nil {
            slog.Error("as-of precompute failed", slog.Any("err", err))
            return 1
        }
    }

    // Then run sequence features
    params := seqcalc.Params{FormatCode: opts.Format}
    if s := strings.TrimSpace(opts.AsOf); s != "" {
        if t, perr := time.Parse("2006-01-02", s); perr == nil {
            params.AsOf = t
        } else {
            // Should not happen because we parsed earlier when needed; log and continue without as-of
            slog.Warn("invalid -as-of for seq stage; ignoring", slog.String("as_of", s), slog.Any("err", perr))
        }
    }

    // Build registry similar to the dedicated command
    reg := seqcalc.NewRegistry(append(
        seqcalc.NewNoopCalculators(),
        seqcalc.NewBatTransitionsCalculator(),
        seqcalc.NewBowlSequencesCalculator(),
        seqcalc.NewPlayerWindowsCalculator(),
        seqcalc.NewReactionCalculator(),
        seqcalc.NewDotStreaksCalculator(),
        seqcalc.NewDisciplineCalculator(),
        seqcalc.NewWicketModesCalculator(),
        seqcalc.NewSpellsCalculator(),
        seqcalc.NewOverPosCalculator(),
        seqcalc.NewEndPressureCalculator(),
    )...)

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
