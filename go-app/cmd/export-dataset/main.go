// Command export-dataset exports training CSV datasets from the database.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	exportrepo "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/db/exportrepo"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/fsx/osfs"
	exportcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	expcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/exportdataset"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	exportsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/exportdataset"
)

func main() {
	// Phase 2: delegate flag parsing and output dir preparation to internal packages.
	fs := flag.NewFlagSet("export-dataset", flag.ContinueOnError)
	opts, err := exportcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		// Preserve legacy behavior: print error and exit similar to flag.Parse failure.
		slog.Error("flag parsing failed", slog.Any("err", err))
		os.Exit(2)
	}
	// Ensure output directory precedence: flag > env (handled by parser) > config.DefaultExportDir()
	if opts.OutDir == "" {
		opts.OutDir = config.DefaultExportDir()
	}

	// Map options to legacy variables used later in this file while we migrate logic incrementally.
	outDir := opts.OutDir

	logger.SetupFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Prepare filesystem via internal runner (creates outDir). Remove direct os.MkdirAll.

	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}

	// Wire internal services and runner to handle unified, legacy combined, and inference-only flows.
	fsys := osfs.New()
	repo := exportrepo.New()
	bat := exportsvc.NewBattingService(repo)
	bow := exportsvc.NewBowlingService(repo)
	runner := expcmd.NewRunnerWithServices(fsys, bat, bow)
	if runErr := runner.Run(ctx, opts); runErr != nil {
		slog.Error("runner execution failed", slog.Any("err", runErr))
		os.Exit(1)
	}
	// All flows are handled by Runner; log and return.
	slog.Info("exports written", slog.String("dir", outDir))
}
