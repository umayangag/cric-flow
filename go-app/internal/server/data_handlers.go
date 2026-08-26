package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// fetchCommand is the data_migrations command for a dataset download. It is the
// registry's command for the "fetch" step, not a literal chosen here.
var fetchCommand = mustStepCommand("fetch")

// dataFeedsHandler handles GET /ops/data/feeds.
//
// It returns the named feeds and the host allowlist together, so the UI can state
// the rule up front rather than let an operator discover it by being refused.
func (a *App) dataFeedsHandler(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"feeds":         dataacquire.Feeds(),
		"allowed_hosts": dataacquire.AllowedHosts(),
		"staging_dir":   dataset.StagingDir(),
	})
}

// dataFetchRequest is the body of POST /ops/data/fetch. Exactly one of Feed and URL.
type dataFetchRequest struct {
	Feed string `json:"feed"`
	URL  string `json:"url"`
}

// dataFetchHandler handles POST /ops/data/fetch: it downloads a Cricsheet archive
// into the staging directory as a tracked background job.
//
// It answers 202 rather than holding the request open. A multi-hundred-megabyte
// archive over a slow link outlives any sane HTTP timeout, and a request-scoped
// download would be cancelled by the proxy mid-transfer with nothing recorded —
// which is why every long step in this service is a tracked job.
func (a *App) dataFetchHandler(w http.ResponseWriter, r *http.Request) {
	var body dataFetchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Info("data fetch: decode request body failed", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}

	src, err := dataacquire.ResolveSource(body.Feed, body.URL)
	if err != nil {
		slog.Info("data fetch: rejected source",
			slog.String("feed", body.Feed), slog.String("url", body.URL), slog.Any("err", err))
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"error":         err.Error(),
			"allowed_hosts": dataacquire.AllowedHosts(),
		})
		return
	}

	if busy, _ := pipeline.LaneBusy(r.Context(), fetchCommand); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": pipeline.ErrPipelineBusy.Error()})
		return
	}

	stagingDir := dataset.StagingDir()
	args := map[string]any{
		"feed":        src.FeedID,
		"url":         src.URL.Redacted(),
		"filename":    src.Filename,
		"staging_dir": stagingDir,
	}
	slog.Info("data fetch: request accepted, starting background job", slog.Any("args", args))

	a.startTrackedJob(fetchCommand, args, dataacquire.DefaultTimeout,
		func(ctx context.Context) (any, error) {
			// Clearing on every exit — success, failure or cancellation — is what
			// stops the console from showing a transfer that is no longer running.
			defer dataacquire.ClearProgress()
			result, err := dataacquire.Fetch(ctx, src, dataacquire.Options{
				StagingDir: stagingDir,
				Progress: func(p dataacquire.Progress) {
					dataacquire.PublishProgress(src.URL.Redacted(), p)
				},
			})
			if err != nil {
				return nil, err
			}
			return result, nil
		})

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":   "started",
		"step":     "fetch",
		"feed":     src.FeedID,
		"url":      src.URL.Redacted(),
		"filename": src.Filename,
	})
}

// mustStepCommand returns a registry step's command, panicking at startup if the step
// is missing. A typo here would otherwise surface as a job recorded under a command
// no lane recognises — which LaneForCommand would then treat as LaneCompute, quietly
// making the download block training. Failing at init is the cheaper failure.
func mustStepCommand(stepID string) string {
	step, ok := pipelinesvc.Steps().ByID(stepID)
	if !ok {
		panic("pipeline registry has no step " + strings.TrimSpace(stepID))
	}
	return step.Command
}
