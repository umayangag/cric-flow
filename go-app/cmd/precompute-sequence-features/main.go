package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/seqcalc"
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

	// Build registry: register no-ops first, then override with real calculators where available
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
	calcs, err := reg.ResolveTargets(*targets)
	if err != nil {
		return err
	}

	if *dryRun {
		return seqcalc.DryRun(out, calcs, params)
	}
	ctx := context.Background()
	return seqcalc.Run(ctx, calcs, params, false)
}
