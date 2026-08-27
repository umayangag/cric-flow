package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/umayangag/cric-flow/go-app/internal/services/runplan"
)

// runPlanRequest is the body of POST /ops/pipeline/run-plan.
//
// A named plan or an explicit step list, never both: the backend refuses the ambiguity
// rather than picking one, the same way /ops/data/fetch refuses a feed and a URL
// together.
type runPlanRequest struct {
	Plan  string   `json:"plan"`
	Steps []string `json:"steps"`
}

// runPlanStartHandler handles POST /ops/pipeline/run-plan.
//
// This is the API equivalent of `make up-all` / `make full-pipeline`, which existed
// only in the Makefile: from the console the same thing was eleven manual clicks with
// waiting in between.
//
// It answers 202. A full pipeline outlives any request by hours, and the plan's state
// is persisted as it goes — so the browser that started it can be closed without
// losing it.
func (a *App) runPlanStartHandler(w http.ResponseWriter, r *http.Request) {
	var body runPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Info("run plan: decode request body failed", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}
	// An empty body means the obvious thing rather than an error.
	if body.Plan == "" && len(body.Steps) == 0 {
		body.Plan = runplan.PlanFull
	}

	steps, err := runplan.Resolve(body.Plan, body.Steps)
	if err != nil {
		slog.Info("run plan: rejected", slog.String("plan", body.Plan), slog.Any("err", err))
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
			"plans": runplan.Names(),
		})
		return
	}

	if _, _, running, activeErr := (runplan.TrackingStore{}).Active(r.Context()); activeErr == nil && running {
		respondJSON(w, http.StatusConflict, map[string]string{"error": runplan.ErrPlanRunning.Error()})
		return
	}

	stepIDs := make([]string, 0, len(steps))
	for _, step := range steps {
		stepIDs = append(stepIDs, step.ID)
	}
	slog.Info("run plan: starting", slog.String("plan", body.Plan), slog.Any("steps", stepIDs))
	a.StartRunPlan(body.Plan, steps)

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"plan":   body.Plan,
		"steps":  stepIDs,
	})
}

// runPlanStateHandler handles GET /ops/pipeline/plan.
//
// It returns the most recent plan, running or not. That is what lets the console show
// a plan it did not start — after a page reload, from another tab, or the morning
// after the browser was closed — because the state lives in the database rather than
// in whichever page happened to trigger it.
func (a *App) runPlanStateHandler(w http.ResponseWriter, r *http.Request) {
	store := runplan.TrackingStore{}

	id, state, found, err := store.Latest(r.Context())
	if err != nil {
		slog.Error("run plan: reading state failed", slog.Any("err", err))
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read the plan state"})
		return
	}
	if !found {
		respondJSON(w, http.StatusOK, map[string]any{"running": false, "plans": runplan.Names()})
		return
	}

	_, _, running, _ := store.Active(r.Context())
	payload := map[string]any{
		"id":      id,
		"running": running,
		"plan":    state.Plan,
		"steps":   state.Steps,
		"plans":   runplan.Names(),
	}
	if state.StartedAt != "" {
		payload["started_at"] = state.StartedAt
	}
	if state.FinishedAt != "" {
		payload["finished_at"] = state.FinishedAt
	}
	// Where a resume would start. Reported rather than computed by the client, so the
	// answer comes from the same place the executor's own skip logic does.
	if next, ok := state.FirstIncomplete(); ok && !running {
		payload["resume_from"] = next.StepID
	}
	respondJSON(w, http.StatusOK, payload)
}
