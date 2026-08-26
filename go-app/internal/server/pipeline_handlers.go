package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	exportsvc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/services/opsstatus"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// pipelineStopHandler handles POST /ops/pipeline/stop. It cancels the job contexts of
// the requested lanes and marks their in-flight runs CANCELLED.
//
// An optional ?lane= names one lane ("compute" or "data"); with no lane it stops
// everything, which is what the Stop button means. The parameter exists because the
// lanes overlap by design: cancelling a ten-minute download should not have to also
// abandon a training run that has been going for eight.
func (a *App) pipelineStopHandler(w http.ResponseWriter, r *http.Request) {
	lanes, err := requestedLanes(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cancelled, err := pipelinesvc.StopRun(
		r.Context(),
		"cancelled by user",
		lanes,
		a.CancelJobsInLanes,
		tracking.CancelInProgressMigrations,
	)
	if err != nil {
		slog.Warn("pipeline stop: cancel migration failed", slog.Any("err", err))
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cancelled == 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "no pipeline step is running"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"status": "cancelled", "cancelled": cancelled})
}

// requestedLanes reads the optional ?lane= parameter. Nil means every lane.
func requestedLanes(r *http.Request) ([]pipelinesvc.Lane, error) {
	raw := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("lane")))
	if raw == "" {
		return nil, nil
	}
	lane := pipelinesvc.Lane(raw)
	if !pipelinesvc.IsKnownLane(lane) {
		return nil, fmt.Errorf("unknown lane %q; known lanes: %s",
			raw, strings.Join(pipelinesvc.LaneNames(), ", "))
	}
	return []pipelinesvc.Lane{lane}, nil
}

// pipelineRunHandler handles POST /ops/pipeline/run/{step}.
//
// The set of steps it accepts is exactly pipelinesvc.Steps() — there is no list here
// to fall out of step with the registry, with /ops/status, or with the frontend.
// Order is enforced before dispatch: a step whose prerequisites have not completed,
// or one already running, is refused with 409 rather than started.
func (a *App) pipelineRunHandler(w http.ResponseWriter, r *http.Request) {
	stepID := strings.TrimSpace(strings.ToLower(mux.Vars(r)["step"]))
	if stepID == "" {
		slog.Info("pipeline run: empty step")
		respondBadRequest(w, nil)
		return
	}

	step, known := pipelinesvc.Steps().ByID(stepID)
	if !known {
		slog.Info("pipeline run: unknown step", slog.String("step", stepID))
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown step: " + stepID})
		return
	}
	// Acquisition steps are in the registry — that is what keeps their lane and label
	// honest — but they are not stages of the pipeline and are not started from here.
	// Saying where they live beats a bare 400 that leaves the caller guessing.
	if step.EffectiveSurface() != pipelinesvc.SurfacePipeline {
		slog.Info("pipeline run: step is not a pipeline stage", slog.String("step", step.ID))
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": step.Label + " is a dataset step, not a pipeline stage; start it with POST /ops/data/" + step.ID,
		})
		return
	}
	slog.Info("pipeline run requested", slog.String("step", step.ID))

	if ok, msg := opsstatus.CanRunPipelineStep(r.Context(), step.ID); !ok {
		respondJSON(w, http.StatusConflict, map[string]string{"error": msg})
		return
	}

	a.handlerForStep(step)(w, r)
}

// handlerForStep returns the handler that executes a step. Steps run by ml-service
// all share makeMLTrainHandler; the three go-app-native steps have their own.
func (a *App) handlerForStep(step pipelinesvc.Step) http.HandlerFunc {
	switch step.ID {
	case "import":
		return a.importCricSheetHandler
	case "precompute":
		return a.precomputeHandler
	case "export":
		return a.runExportHandler
	}
	if step.RunsOnMLService() {
		return a.makeMLTrainHandler(step)
	}
	// Unreachable while every registry step is either native or ML-backed; the
	// registry test asserts that, so this only fires if someone adds a step and
	// forgets the handler.
	return func(w http.ResponseWriter, _ *http.Request) {
		slog.Error("pipeline run: step has no handler", slog.String("step", step.ID))
		respondJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "step " + step.ID + " has no handler",
		})
	}
}

// runExportHandler starts export-dataset in the background with tracking.
func (a *App) runExportHandler(w http.ResponseWriter, r *http.Request) {
	if busy, _ := pipeline.LaneBusy(r.Context(), "export-dataset"); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": pipeline.ErrPipelineBusy.Error()})
		return
	}

	outDir := config.DefaultExportDir()
	opts := exportsvc.Options{OutDir: outDir, Unified: true}
	a.startTrackedJob("export-dataset", map[string]any{"out_dir": outDir}, config.ExportTimeout(),
		func(ctx context.Context) (any, error) {
			repo := &exportqueries.Repo{}
			runner := exportsvc.NewRunnerWithServices(
				exportsvc.NewBattingService(repo),
				exportsvc.NewBowlingService(repo),
				exportsvc.NewFieldingService(repo),
				exportsvc.NewExtrasService(repo),
				exportsvc.NewWinService(repo),
			)
			return map[string]any{"out_dir": outDir}, runner.Run(ctx, opts)
		})

	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "export"})
}

// autoTuneQueryParams are the extra query params auto_tune forwards to ml-service.
// No other step forwards anything beyond cutoff.
var autoTuneQueryParams = []string{"model", "format", "all_formats", "unified", "rescreen", "algorithms"}

// makeMLTrainHandler returns the handler for a step executed by ml-service.
//
// Every such step follows the same shape: resolve the cutoff, confirm the operator
// is happy to train on default parameters when none are tuned, then start a tracked
// background job that POSTs to ml-service. Taking the whole Step rather than three
// loose strings is what keeps the ID, the command and the endpoint from drifting.
func (a *App) makeMLTrainHandler(step pipelinesvc.Step) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		args := map[string]any{"step": step.ID}

		cutoff := strings.TrimSpace(q.Get("cutoff"))
		if cutoff == "" {
			cutoff = pipelinesvc.DefaultCutoff()
		}
		args["cutoff"] = cutoff
		query := url.Values{"cutoff": []string{cutoff}}

		if step.IsTraining() {
			if handled := a.confirmDefaultParams(w, r, step); handled {
				return
			}
		}
		if step.ID == "auto_tune" {
			for _, name := range autoTuneQueryParams {
				if v := strings.TrimSpace(q.Get(name)); v != "" {
					query.Set(name, v)
					args[name] = v
				}
			}
		}

		a.startTrackedJob(step.Command, args, pipelinesvc.TrainStepTimeout(),
			func(ctx context.Context) (any, error) {
				return nil, pipelinesvc.CallMLTrainEndpoint(ctx, step.MLEndpoint, "?"+query.Encode())
			})
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": step.ID})
	}
}

// confirmDefaultParams answers the request itself — and reports true — when the step
// trains a model that has no auto-tuned parameters and the operator has not yet
// confirmed training on config defaults. The client re-posts with confirm_use_default=1.
func (a *App) confirmDefaultParams(w http.ResponseWriter, r *http.Request, step pipelinesvc.Step) bool {
	if db.Pool == nil {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(r.URL.Query().Get("confirm_use_default"))) {
	case "1", "true", "yes":
		return false
	}

	hasParams, err := db.HasAnyTunedParamsForModel(r.Context(), step.Model)
	if err != nil {
		slog.Warn("pipeline: tuned params check failed", "step", step.ID, "model", step.Model, "err", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check tuned params"})
		return true
	}
	if hasParams {
		return false
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"requires_confirmation": true,
		"message":               "No auto-tuned parameters found for this model. Train with default config parameters?",
		"step":                  step.ID,
	})
	return true
}

// startTrackedJob runs work in the background under the app's job context, recording
// it in data_migrations so /ops/status, the SSE stream and run history all see it.
// The caller has already answered the request with 202.
func (a *App) startTrackedJob(
	command string,
	args map[string]any,
	timeout time.Duration,
	work func(context.Context) (any, error),
) {
	lane := pipelinesvc.Steps().LaneForCommand(command)
	jobCtx, cancel := context.WithCancel(a.JobContext())
	a.SetJobCancel(lane, cancel)
	go func() {
		defer a.ClearJobCancel(lane)
		slog.Info(command+" started", slog.Any("args", args))
		if err := pipeline.RunJob(jobCtx, command, args, timeout, work); err != nil {
			slog.Error(command+" failed", slog.Any("err", err))
			return
		}
		slog.Info(command + " completed")
	}()
}
