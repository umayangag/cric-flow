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
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
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

	// The plan goes first: cancelling only the step it is on would stop that step and
	// then let the plan start the next one, which is not what Stop means (ops plan
	// R-1). Cancelling the plan's context also cancels the step beneath it, so the
	// lane cancel below is belt and braces for a step started on its own.
	planStopped := a.StopRunPlan()

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
	if cancelled == 0 && !planStopped {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "no pipeline step is running"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"status":       "cancelled",
		"cancelled":    cancelled,
		"plan_stopped": planStopped,
	})
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
				result, err := pipelinesvc.CallMLTrainEndpointWithResult(ctx, step.MLEndpoint, "?"+query.Encode())
				if err != nil {
					return nil, err
				}
				return trainRunMetadata(step, cutoff, result), nil
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

// trainRunMetadata builds what a finished training step is remembered by, for
// data_migrations.metadata (ops plan O-4).
//
// The dataset digest is stamped here rather than by ml-service, because ml-service
// does not know which dataset produced the CSVs it trained on -- go-app does, from
// the manifest in the dataset directory (A-3). Recording it on the run is what closes
// the provenance loop: "which data produced this model?" becomes a lookup rather than
// an archaeology exercise.
//
// A run with no summary and no live dataset still records the step and cutoff, which
// is more than the previous nil.
func trainRunMetadata(step pipelinesvc.Step, cutoff string, result *pipelinesvc.TrainResult) map[string]any {
	meta := map[string]any{
		"step":   step.ID,
		"cutoff": cutoff,
	}
	if result != nil && len(result.Summary) > 0 {
		meta["summary"] = result.Summary
	}
	if manifest, ok := dataacquire.ReadManifest(dataset.Dir()); ok {
		provenance := map[string]any{}
		if manifest.ArchiveSHA256 != "" {
			provenance["dataset_sha256"] = manifest.ArchiveSHA256
		}
		if manifest.SourceURL != "" {
			provenance["dataset_source_url"] = manifest.SourceURL
		}
		if manifest.FeedID != "" {
			provenance["dataset_feed"] = manifest.FeedID
		}
		if manifest.ExtractedAt != "" {
			provenance["dataset_extracted_at"] = manifest.ExtractedAt
		}
		if manifest.MatchFiles > 0 {
			provenance["dataset_match_files"] = manifest.MatchFiles
		}
		if len(provenance) > 0 {
			meta["provenance"] = provenance
		}
	}
	return meta
}
