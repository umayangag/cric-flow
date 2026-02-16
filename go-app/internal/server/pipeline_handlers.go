package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	exportcli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	expcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/exportdataset"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/pipeline"
	exportsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/exportdataset"
)

// pipelineRunHandler handles POST /ops/pipeline/run/:step.
// Triggers import, precompute, or export in-process; returns 202 started or 501 for train/auto_tune.
func (a *App) pipelineRunHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	step := strings.TrimSpace(strings.ToLower(vars["step"]))
	if step == "" {
		respondBadRequest(w, nil)
		return
	}

	switch step {
	case "import":
		importCricSheetHandler(w, r)
		return
	case "precompute":
		precomputeHandler(w, r)
		return
	case "export":
		a.runExportHandler(w, r)
		return
	case "train_batting", "train_bowling", "train_fielding", "auto_tune":
		respondJSON(w, http.StatusNotImplemented, map[string]string{
			"error":   "step must be run from project root",
			"step":    step,
			"command": stepToCommand(step),
		})
		return
	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown step: " + step})
		return
	}
}

func stepToCommand(step string) string {
	switch step {
	case "train_batting":
		return "make train-batting"
	case "train_bowling":
		return "make train-bowling"
	case "train_fielding":
		return "make train-fielding CUTOFF=2025-01-01T00:00:00Z"
	case "auto_tune":
		return "make ml-auto-tune MODEL=all ALL_FORMATS=1"
	default:
		return ""
	}
}

// runExportHandler starts export-dataset in the background with tracking.
func (a *App) runExportHandler(w http.ResponseWriter, _ *http.Request) {
	outDir := config.DefaultExportDir()
	cfg := config.Load()
	opts := exportcli.Options{
		OutDir:  outDir,
		Unified: true,
	}
	if cfg != nil && cfg.Export.SplitByFormat {
		opts.Formats = []string{"TEST", "ODI", "T20", "T20I"}
	}

	go func() {
		slog.Info("export-dataset started", slog.String("dir", outDir))
		runErr := pipeline.RunJob(
			context.Background(),
			"export-dataset",
			map[string]any{"out_dir": outDir},
			5*time.Minute,
			func(ctx context.Context) (any, error) {
				repo := &exportqueries.Repo{}
				bat := exportsvc.NewBattingService(repo)
				bow := exportsvc.NewBowlingService(repo)
				runner := expcmd.NewRunnerWithServices(bat, bow)
				err := runner.Run(ctx, opts)
				return map[string]any{"out_dir": outDir}, err
			},
		)
		if runErr != nil {
			slog.Error("export-dataset failed", slog.Any("err", runErr))
		} else {
			slog.Info("export-dataset completed", slog.String("dir", outDir))
		}
	}()

	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "export"})
}
