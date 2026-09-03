package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// runReloadBodyLimit caps the reload response we read. It is a manifest summary and a
// list of loaded formats; anything larger is a bug on the other side.
const runReloadBodyLimit = 1 << 20

// reloadRunHandler handles the `reload` pipeline step: point ml-service's `current` at a
// run and load it.
//
// It is go-app-native rather than an /admin/train/* call because reload trains nothing.
// The run is named by ?run_id=; with none, ml-service loads the newest run on disk,
// which is what the step after a retrain means by "reload".
func (a *App) reloadRunHandler(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	args := map[string]any{"step": "reload"}
	query := url.Values{}
	if runID != "" {
		args["run_id"] = runID
		query.Set(pipeline.MLQueryRun, runID)
	}

	a.startTrackedJob("xi-reload", args, pipeline.TrainStepTimeout(),
		func(ctx context.Context) (any, error) {
			return callMLReload(ctx, query)
		})
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started", "step": "reload"})
}

// callMLReload POSTs ml-service /admin/reload and returns what it loaded, so the run's
// data_migrations row records which run is now serving (H-16).
func callMLReload(ctx context.Context, query url.Values) (map[string]any, error) {
	endpoint := pipeline.MLServiceBaseURL() + pipeline.MLReloadPath
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if key := pipeline.AdminAPIKey(); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	client := &http.Client{Timeout: pipeline.TrainStepTimeout()}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, runReloadBodyLimit))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Error("reload: ml-service refused",
			slog.Int("status", resp.StatusCode), slog.String("body", string(body)))
		return nil, fmt.Errorf("ml-service %s: %s — %s", endpoint, resp.Status, strings.TrimSpace(string(body)))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Warn("reload: unreadable ml-service response", slog.Any("err", err))
		return map[string]any{"status": "reloaded"}, nil
	}
	slog.Info("reload: ml-service loaded a run", slog.Any("run", payload["run_id"]))
	return payload, nil
}
