package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	exportcli "github.com/umayangag/cric-flow/go-app/internal/cli/exportdataset"
	expcmd "github.com/umayangag/cric-flow/go-app/internal/commands/exportdataset"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	exportsvc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

// pipelineRunHandler handles POST /ops/pipeline/run/:step.
// Triggers import, precompute, or export in-process; returns 202 started or 501 for train/auto_tune.
// Next step is only runnable after the previous completed successfully (enforced here and in /ops/status runnable).
func (a *App) pipelineRunHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	step := strings.TrimSpace(strings.ToLower(vars["step"]))
	if step == "" {
		slog.Info("pipeline run: empty step")
		respondBadRequest(w, nil)
		return
	}
	slog.Info("pipeline run requested", slog.String("step", step))

	// Enforce order: only allow run if previous step completed (and no other step is running).
	switch step {
	case "import", "precompute", "export", "train_batting", "train_bowling", "train_fielding", "train_extras", "train_win":
		if ok, msg := CanRunPipelineStep(r.Context(), step); !ok {
			respondJSON(w, http.StatusConflict, map[string]string{"error": msg})
			return
		}
	}
	if step == "auto_tune" {
		if ok, msg := CanRunPipelineStep(r.Context(), step); !ok {
			respondJSON(w, http.StatusConflict, map[string]string{"error": msg})
			return
		}
	}

	switch step {
	case "import":
		a.importCricSheetHandler(w, r)
		return
	case "precompute":
		a.precomputeHandler(w, r)
		return
	case "export":
		a.runExportHandler(w, r)
		return
	case "train_batting":
		a.runTrainBattingHandler(w, r)
		return
	case "train_bowling":
		a.runTrainBowlingHandler(w, r)
		return
	case "train_fielding":
		a.runTrainFieldingHandler(w, r)
		return
	case "train_extras":
		a.runTrainExtrasHandler(w, r)
		return
	case "train_win":
		a.runTrainWinHandler(w, r)
		return
	case "auto_tune":
		respondJSON(w, http.StatusNotImplemented, map[string]string{
			"error":   "step must be run from project root",
			"step":    step,
			"command": stepToCommand(step),
		})
		return
	default:
		slog.Info("pipeline run: unknown step", slog.String("step", step))
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
	case "train_extras":
		return "make train-extras CUTOFF=2025-01-01T00:00:00Z"
	case "train_win":
		return "make train-win CUTOFF=2025-01-01T00:00:00Z"
	case "auto_tune":
		return "make ml-auto-tune MODEL=all ALL_FORMATS=1"
	default:
		return ""
	}
}

// runExportHandler starts export-dataset in the background with tracking.
func (a *App) runExportHandler(w http.ResponseWriter, r *http.Request) {
	outDir := config.DefaultExportDir()
	cfg := config.Load()
	opts := exportcli.Options{
		OutDir:  outDir,
		Unified: true,
	}
	if cfg != nil && cfg.Export.SplitByFormat {
		opts.Formats = []string{"TEST", "ODI", "T20", "T20I"}
	}
	if busy, _ := pipeline.HasPipelineBusy(r.Context()); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "another pipeline step is already running"})
		return
	}

	go func() {
		slog.Info("export-dataset started", slog.String("dir", outDir))
		runErr := pipeline.RunJob(
			a.JobContext(),
			"export-dataset",
			map[string]any{"out_dir": outDir},
			config.PipelineTimeout(),
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
			slog.Error("export-dataset failed", slog.String("out_dir", outDir), slog.Any("err", runErr))
		} else {
			slog.Info("export-dataset completed", slog.String("dir", outDir))
		}
	}()
	slog.Info("export-dataset job started", slog.String("out_dir", outDir))

	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "export"})
}

// trainStepTimeout is the max time to wait for ML service train endpoint (training can take many minutes).
const trainStepTimeout = 30 * time.Minute

func mlServiceBaseURL() string {
	s := strings.TrimSpace(os.Getenv("ML_SERVICE_URL"))
	if s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return "http://localhost:8000"
}

// callMLTrainEndpoint POSTs to ML service /admin/train/{step} and returns an error on non-2xx or context cancel.
func callMLTrainEndpoint(ctx context.Context, step string, querySuffix string) error {
	base := mlServiceBaseURL()
	url := base + "/admin/train/" + step + querySuffix
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: trainStepTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ml-service %s: %s", url, resp.Status)
	}
	return nil
}

func (a *App) runTrainBattingHandler(w http.ResponseWriter, r *http.Request) {
	go func() {
		slog.Info("train-batting started")
		runErr := pipeline.RunJob(
			a.JobContext(),
			"train-batting",
			map[string]any{"step": "train_batting"},
			trainStepTimeout,
			func(ctx context.Context) (any, error) {
				return nil, callMLTrainEndpoint(ctx, "batting", "")
			},
		)
		if runErr != nil {
			slog.Error("train-batting failed", slog.Any("err", runErr))
		} else {
			slog.Info("train-batting completed")
		}
	}()
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "train_batting"})
}

func (a *App) runTrainBowlingHandler(w http.ResponseWriter, r *http.Request) {
	go func() {
		slog.Info("train-bowling started")
		runErr := pipeline.RunJob(
			a.JobContext(),
			"train-bowling",
			map[string]any{"step": "train_bowling"},
			trainStepTimeout,
			func(ctx context.Context) (any, error) {
				return nil, callMLTrainEndpoint(ctx, "bowling", "")
			},
		)
		if runErr != nil {
			slog.Error("train-bowling failed", slog.Any("err", runErr))
		} else {
			slog.Info("train-bowling completed")
		}
	}()
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "train_bowling"})
}

func defaultCutoff() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (a *App) runTrainFieldingHandler(w http.ResponseWriter, r *http.Request) {
	cutoff := r.URL.Query().Get("cutoff")
	if cutoff == "" {
		cutoff = defaultCutoff()
	}
	go func() {
		slog.Info("train-fielding started", slog.String("cutoff", cutoff))
		runErr := pipeline.RunJob(
			a.JobContext(),
			"train-fielding",
			map[string]any{"step": "train_fielding", "cutoff": cutoff},
			trainStepTimeout,
			func(ctx context.Context) (any, error) {
				q := "?cutoff=" + url.QueryEscape(strings.TrimSpace(cutoff))
				return nil, callMLTrainEndpoint(ctx, "fielding", q)
			},
		)
		if runErr != nil {
			slog.Error("train-fielding failed", slog.Any("err", runErr))
		} else {
			slog.Info("train-fielding completed")
		}
	}()
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "train_fielding"})
}

func (a *App) runTrainExtrasHandler(w http.ResponseWriter, r *http.Request) {
	cutoff := r.URL.Query().Get("cutoff")
	if cutoff == "" {
		cutoff = defaultCutoff()
	}
	go func() {
		slog.Info("train-extras started", slog.String("cutoff", cutoff))
		runErr := pipeline.RunJob(
			a.JobContext(),
			"train-extras",
			map[string]any{"step": "train_extras", "cutoff": cutoff},
			trainStepTimeout,
			func(ctx context.Context) (any, error) {
				q := "?cutoff=" + url.QueryEscape(strings.TrimSpace(cutoff))
				return nil, callMLTrainEndpoint(ctx, "extras", q)
			},
		)
		if runErr != nil {
			slog.Error("train-extras failed", slog.Any("err", runErr))
		} else {
			slog.Info("train-extras completed")
		}
	}()
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "train_extras"})
}

func (a *App) runTrainWinHandler(w http.ResponseWriter, r *http.Request) {
	cutoff := r.URL.Query().Get("cutoff")
	if cutoff == "" {
		cutoff = defaultCutoff()
	}
	go func() {
		slog.Info("train-win started", slog.String("cutoff", cutoff))
		runErr := pipeline.RunJob(
			a.JobContext(),
			"train-win",
			map[string]any{"step": "train_win", "cutoff": cutoff},
			trainStepTimeout,
			func(ctx context.Context) (any, error) {
				q := "?cutoff=" + url.QueryEscape(strings.TrimSpace(cutoff))
				return nil, callMLTrainEndpoint(ctx, "win", q)
			},
		)
		if runErr != nil {
			slog.Error("train-win failed", slog.Any("err", runErr))
		} else {
			slog.Info("train-win completed")
		}
	}()
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "train_win"})
}
