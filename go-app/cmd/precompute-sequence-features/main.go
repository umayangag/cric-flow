package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/seqcalc"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	// Re-parse flags for testability using a new FlagSet
	fs := flag.NewFlagSet("precompute-sequence-features", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	format := fs.String("format", "T20", "match format: T20|ODI|TEST")
	targets := fs.String("targets", "all", "comma-separated targets or 'all'")
	dryRun := fs.Bool("dry-run", false, "list computations without writing")
	asOfStr := fs.String("as-of", "", "as-of date (YYYY-MM-DD); optional")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Basic validation
	f := *format
	if f == "" {
		return fmt.Errorf("-format is required")
	}

	params := seqcalc.Params{FormatCode: f}
	if s := *asOfStr; s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			params.AsOf = t
		} else {
			return fmt.Errorf("invalid -as-of date: %v", err)
		}
	}

	// Build default registry via shared helper to avoid duplication between commands
	reg := seqcalc.NewDefaultRegistry()
	calcs, err := reg.ResolveTargets(*targets)
	if err != nil {
		return err
	}

	if *dryRun {
		return seqcalc.DryRun(out, calcs, params)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Establish DB connection so calculators can write results.
	if _, err := db.Connect(ctx); err != nil {
		return fmt.Errorf("db connect failed: %w", err)
	}

	tracker, tErr := tracking.Start(ctx, "precompute-sequence-features", map[string]interface{}{
		"format":  f,
		"targets": *targets,
		"as_of":   *asOfStr,
	})
	if tErr != nil {
		slog.Warn("tracking start failed", slog.Any("err", tErr))
	}

	var runErr error
	defer func() {
		tracker.CaptureExit(ctx, &runErr, nil)
	}()

	if runErr = seqcalc.Run(ctx, calcs, params, false); runErr != nil {
		return runErr
	}
	return nil
}
