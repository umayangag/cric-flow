package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	exportcli "github.com/umayangag/cric-flow/go-app/internal/cli/exportdataset"
	expcmd "github.com/umayangag/cric-flow/go-app/internal/commands/exportdataset"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	formatsPkg "github.com/umayangag/cric-flow/go-app/internal/formats"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	exportsvc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// pipelineStopHandler handles POST /ops/pipeline/stop. Cancels the current pipeline job context and marks the in-progress migration as CANCELLED.
func (a *App) pipelineStopHandler(w http.ResponseWriter, r *http.Request) {
	cancelled, err := pipelinesvc.StopRun(
		r.Context(),
		"cancelled by user",
		a.CancelCurrentJob,
		tracking.CancelInProgressMigration,
	)
	if err != nil {
		slog.Warn("pipeline stop: cancel migration failed", slog.Any("err", err))
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !cancelled {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "no pipeline step is running"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// pipelineRunHandler handles POST /ops/pipeline/run/:step.
// Triggers import, precompute, export, train_*, or auto_tune (train/auto_tune via ML service); returns 202 started or 501 for train_combination_meta.
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
	case "import",
		"precompute",
		"export",
		"train_batting",
		"train_bowling",
		"train_fielding",
		"train_extras",
		"train_win",
		"train_innings",
		"train_combination_meta":
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
		a.makeMLTrainHandler("train_batting", "train-batting", "batting")(w, r)
		return
	case "train_bowling":
		a.makeMLTrainHandler("train_bowling", "train-bowling", "bowling")(w, r)
		return
	case "train_fielding":
		a.makeMLTrainHandler("train_fielding", "train-fielding", "fielding")(w, r)
		return
	case "train_extras":
		a.makeMLTrainHandler("train_extras", "train-extras", "extras")(w, r)
		return
	case "train_win":
		a.makeMLTrainHandler("train_win", "train-win", "win")(w, r)
		return
	case "train_innings":
		a.makeMLTrainHandler("train_innings", "train-innings", "innings")(w, r)
		return
	case "train_combination_meta":
		// Run from project root: make train-combination-meta CSV=<path> OUT=<path>
		exportDir := config.DefaultExportDir()
		csvPath := filepath.Join(exportDir, "backtest_contributions.csv")
		outPath := filepath.Join(exportDir, "combination_meta.json")
		if cfg := config.Load(); cfg != nil && cfg.Selection.MetaModelPath != "" {
			outPath = cfg.Selection.MetaModelPath
		}
		respondJSON(w, http.StatusNotImplemented, map[string]string{
			"error":   "step must be run from project root",
			"step":    step,
			"command": fmt.Sprintf("make train-combination-meta CSV=%s OUT=%s", csvPath, outPath),
		})
		return
	case "auto_tune":
		a.makeMLTrainHandler("auto_tune", "ml-auto-tune", "auto-tune")(w, r)
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
		return "make train-batting CUTOFF=2025-01-01T00:00:00Z"
	case "train_bowling":
		return "make train-bowling CUTOFF=2025-01-01T00:00:00Z"
	case "train_fielding":
		return "make train-fielding CUTOFF=2025-01-01T00:00:00Z"
	case "train_extras":
		return "make train-extras CUTOFF=2025-01-01T00:00:00Z"
	case "train_win":
		return "make train-win CUTOFF=2025-01-01T00:00:00Z"
	case "train_innings":
		return "make train-innings CUTOFF=2025-01-01T00:00:00Z"
	case "train_combination_meta":
		return "make train-combination-meta CSV=<export_dir>/backtest_contributions.csv OUT=<export_dir>/combination_meta.json"
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
		opts.Formats = formatsPkg.CanonicalCodes()
	}
	if busy, _ := pipeline.HasPipelineBusy(r.Context()); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "another pipeline step is already running"})
		return
	}

	jobCtx, cancel := context.WithCancel(a.JobContext())
	a.SetCurrentJobCancel(cancel)
	go func() {
		defer a.ClearCurrentJobCancel()
		slog.Info("export-dataset started", slog.String("dir", outDir))
		runErr := pipeline.RunJob(
			jobCtx,
			"export-dataset",
			map[string]any{"out_dir": outDir},
			config.ExportTimeout(),
			func(ctx context.Context) (any, error) {
				repo := &exportqueries.Repo{}
				bat := exportsvc.NewBattingService(repo)
				bow := exportsvc.NewBowlingService(repo)
				field := exportsvc.NewFieldingService(repo)
				extras := exportsvc.NewExtrasService(repo)
				win := exportsvc.NewWinService(repo)
				runner := expcmd.NewRunnerWithServices(bat, bow, field, extras, win)
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

func trainStepTimeout() time.Duration {
	mins := config.ServerTrainStepTimeoutMin(config.Load())
	return time.Duration(mins) * time.Minute
}

func mlServiceBaseURL() string {
	s := strings.TrimSpace(os.Getenv("ML_SERVICE_URL"))
	if s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return config.ServerMLBaseURLFallback(config.Load())
}

// callMLTrainEndpoint POSTs to ML service /admin/train/{step} and returns an error on non-2xx or context cancel.
// When ml-service ADMIN_API_KEY is set, send X-API-Key (use ML_SERVICE_ADMIN_API_KEY or API_KEY so it matches).
func callMLTrainEndpoint(ctx context.Context, step string, querySuffix string) error {
	base := mlServiceBaseURL()
	url := base + "/admin/train/" + step + querySuffix
	slog.Info("pipeline: calling ML service train endpoint",
		slog.String("step", step),
		slog.String("url", url))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	if key := strings.TrimSpace(os.Getenv("ML_SERVICE_ADMIN_API_KEY")); key != "" {
		req.Header.Set("X-API-Key", key)
	} else if key := strings.TrimSpace(os.Getenv("API_KEY")); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	client := &http.Client{Timeout: trainStepTimeout()}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		msg := string(body)
		if msg != "" {
			return fmt.Errorf("ml-service %s: %s — %s", url, resp.Status, msg)
		}
		return fmt.Errorf("ml-service %s: %s", url, resp.Status)
	}
	return nil
}

func defaultCutoff() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// trainingStepToModel maps pipeline train step ID to ml model name for tuned-params lookup.
func trainingStepToModel(stepID string) string {
	switch stepID {
	case "train_batting":
		return "batting"
	case "train_bowling":
		return "bowling"
	case "train_fielding":
		return "fielding"
	case "train_extras":
		return "extras"
	case "train_innings":
		return "innings"
	case "train_win":
		return "win"
	default:
		return ""
	}
}

// makeMLTrainHandler creates a handler for a training pipeline step that calls an ML service endpoint.
// For auto_tune, forwards query params: model, format, all_formats, unified.
// For train_* steps: if no auto-tuned params exist in DB and confirm_use_default is not set, returns 200 with
// requires_confirmation so the UI can prompt; if user confirms, client re-posts with confirm_use_default=1.
func (a *App) makeMLTrainHandler(stepID, command, mlEndpoint string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args := map[string]any{"step": stepID}
		q := r.URL.Query()
		cutoff := q.Get("cutoff")
		if cutoff == "" {
			cutoff = defaultCutoff()
		}
		args["cutoff"] = cutoff
		querySuffix := "?cutoff=" + url.QueryEscape(strings.TrimSpace(cutoff))

		// Training steps: require user confirmation when no tuned params in DB (unless confirm_use_default is set).
		if model := trainingStepToModel(stepID); model != "" {
			confirmVal := strings.TrimSpace(strings.ToLower(q.Get("confirm_use_default")))
			confirmUseDefault := confirmVal == "1" || confirmVal == "true" || confirmVal == "yes"
			if !confirmUseDefault && db.Pool != nil {
				hasParams, err := db.HasAnyTunedParamsForModel(r.Context(), model)
				if err != nil {
					slog.Warn("pipeline: tuned params check failed", "step", stepID, "model", model, "err", err)
					respondJSON(
						w,
						http.StatusInternalServerError,
						map[string]string{"error": "failed to check tuned params"},
					)
					return
				}
				if !hasParams {
					respondJSON(w, http.StatusOK, map[string]any{
						"requires_confirmation": true,
						"message":               "No auto-tuned parameters found for this model. Train with default config parameters?",
						"step":                  stepID,
					})
					return
				}
			}
		}

		if stepID == "auto_tune" {
			if v := strings.TrimSpace(q.Get("model")); v != "" {
				querySuffix += "&model=" + url.QueryEscape(v)
				args["model"] = v
			}
			if v := strings.TrimSpace(q.Get("format")); v != "" {
				querySuffix += "&format=" + url.QueryEscape(v)
				args["format"] = v
			}
			if v := strings.TrimSpace(q.Get("all_formats")); v != "" {
				querySuffix += "&all_formats=" + url.QueryEscape(v)
				args["all_formats"] = v
			}
			if v := strings.TrimSpace(q.Get("unified")); v != "" {
				querySuffix += "&unified=" + url.QueryEscape(v)
				args["unified"] = v
			}
			if v := strings.TrimSpace(q.Get("rescreen")); v != "" {
				querySuffix += "&rescreen=" + url.QueryEscape(v)
				args["rescreen"] = v
			}
			if v := strings.TrimSpace(q.Get("algorithms")); v != "" {
				querySuffix += "&algorithms=" + url.QueryEscape(v)
				args["algorithms"] = v
			}
		}

		jobCtx, cancel := context.WithCancel(a.JobContext())
		a.SetCurrentJobCancel(cancel)
		go func() {
			defer a.ClearCurrentJobCancel()
			slog.Info(command+" started", slog.Any("args", args))
			runErr := pipeline.RunJob(
				jobCtx,
				command,
				args,
				trainStepTimeout(),
				func(ctx context.Context) (any, error) {
					return nil, callMLTrainEndpoint(ctx, mlEndpoint, querySuffix)
				},
			)
			if runErr != nil {
				slog.Error(command+" failed", slog.Any("err", runErr))
			} else {
				slog.Info(command + " completed")
			}
		}()
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": stepID})
	}
}
