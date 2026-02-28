// Command export-dataset exports training CSV datasets from the database.
// Uses pipeline.RunJob (shared with pipeline handler) for panic recovery and tracking.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	exportcli "github.com/umayangag/cric-flow/go-app/internal/cli/exportdataset"
	expcmd "github.com/umayangag/cric-flow/go-app/internal/commands/exportdataset"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	exportsvc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

func main() { os.Exit(run()) }

func run() int {
	fs := flag.NewFlagSet("export-dataset", flag.ContinueOnError)
	opts, err := exportcli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		return 2
	}
	if opts.OutDir == "" {
		opts.OutDir = config.DefaultExportDir()
	}
	outDir := opts.OutDir

	logger.SetupFromEnv()

	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(baseCtx, 90*time.Second)
	defer cancel()

	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}

	startMeta := map[string]any{"out_dir": outDir}
	runErr := pipeline.RunJob(ctx, "export-dataset", startMeta, 0, func(jobCtx context.Context) (any, error) {
		repo := &exportqueries.Repo{}
		bat := exportsvc.NewBattingService(repo)
		bow := exportsvc.NewBowlingService(repo)
		field := exportsvc.NewFieldingService(repo)
		extras := exportsvc.NewExtrasService(repo)
		win := exportsvc.NewWinService(repo)
		runner := expcmd.NewRunnerWithServices(bat, bow, field, extras, win)
		err := runner.Run(jobCtx, opts)
		return map[string]any{"out_dir": outDir}, err
	})
	if runErr != nil {
		slog.Error("runner execution failed", slog.Any("err", runErr))
		return 1
	}
	slog.Info("exports written", slog.String("dir", outDir))
	return 0
}
