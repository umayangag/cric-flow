package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// StopTrainingTimeout bounds the wait for ml-service to stop its training subprocess.
//
// It is generous because the far side does not answer until the process is gone, and a
// process is given TERMINATE_GRACE_SEC (10s) to exit on SIGTERM before it is killed. A
// stop that returned before that would be back to reporting an intention as a fact — the
// whole of D-10 — so this waits for the truth rather than truncating it.
func StopTrainingTimeout() time.Duration { return 30 * time.Second }

// stopTrainingResponse is ml-service's answer, of which go-app reads exactly one field.
type stopTrainingResponse struct {
	// Stopped lists the steps whose process ml-service watched exit. The JSON tag is
	// StopResponseField, declared on the boundary and asserted from both sides (H-24).
	Stopped []string `json:"stopped"`
}

// StopMLTraining asks ml-service to stop every training step it is running and returns
// the ones it confirms are stopped.
//
// An empty list with no error means nothing was running there, which is the honest answer
// to a Stop pressed when the compute lane is idle — not a failure. An error means the
// question could not be answered, and a caller must not report the run as stopped: that
// is the state D-10 was in every time.
func StopMLTraining(ctx context.Context) ([]string, error) {
	url := MLServiceBaseURL() + MLStopPath
	// A stop must not inherit the caller's cancellation. The request that is being
	// stopped is usually the one whose context is about to be cancelled, and a stop that
	// dies with it is a stop that never happened.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), StopTrainingTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	if key := AdminAPIKey(); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ask ml-service to stop training: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, mlErrorBodyLimit))
		return nil, newMLError(url, resp.StatusCode, resp.Status, body)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, mlErrorBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read stop response: %w", err)
	}
	var parsed stopTrainingResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Unlike a run summary, this body cannot be shrugged off: it is the only
		// evidence the process is gone, and without it we do not know.
		return nil, fmt.Errorf("unreadable stop response from ml-service: %w", err)
	}
	slog.Info("pipeline stop: ml-service stopped training",
		slog.Any("steps", parsed.Stopped))
	return parsed.Stopped, nil
}
