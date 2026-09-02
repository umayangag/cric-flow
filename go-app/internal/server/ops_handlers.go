package server

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

type OpsHandler struct{}

func (h *OpsHandler) ListMigrations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	cfg := config.Load()
	pageCap := config.OpsMigrationsPageCap(cfg)
	if page > pageCap {
		slog.Warn("pagination page capped", "requested", page, "capped_at", pageCap)
		page = pageCap
	}

	limit := config.OpsMigrationsPageDefault(cfg)
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	maxLimit := config.OpsMigrationsPageMax(cfg)
	if limit > maxLimit {
		slog.Warn("pagination limit capped", "requested", limit, "capped_at", maxLimit)
		limit = maxLimit
	}

	offset := (page - 1) * limit

	migrations, total, err := tracking.GetMigrationsPaginated(ctx, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type listResponse struct {
		Items []tracking.Migration `json:"items"`
		Total int                  `json:"total"`
		Page  int                  `json:"page"`
		Limit int                  `json:"limit"`
	}
	resp := listResponse{
		Items: migrations,
		Total: total,
		Page:  page,
		Limit: limit,
	}

	writeJSON(w, http.StatusOK, resp)
}

type Suggestion struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command"`
	Priority    string `json:"priority"` // HIGH, MEDIUM, LOW
}

func (h *OpsHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	migrations, err := tracking.GetRecentMigrations(ctx, config.OpsRecentMigrationsCount(config.Load()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	suggestions := GenerateSuggestions(migrations)

	writeJSON(w, http.StatusOK, suggestions)
}

// GenerateSuggestions names the next step the run history says is missing.
//
// The chain is the pipeline: import, then retrain, then reload. Each rule fires when a
// step has never run or last ran before the step it depends on, which is the same
// ordering the registry's Requires graph states -- read here from what actually
// happened rather than from what is configured.
func GenerateSuggestions(migrations []tracking.Migration) []Suggestion {
	lastRuns := make(map[string]*tracking.Migration)
	for i := range migrations {
		m := &migrations[i]
		if m.Status != tracking.StatusCompleted {
			continue
		}
		// Only store the first completed migration found for each command,
		// since the list is sorted by most recent.
		if _, ok := lastRuns[m.Command]; !ok {
			lastRuns[m.Command] = m
		}
	}

	lastImport := lastRuns["cricsheet-import"]
	lastRetrain := lastRuns["xi-retrain"]
	lastReload := lastRuns["xi-reload"]

	if lastImport == nil {
		return []Suggestion{{
			Title:       "Initialize Data",
			Description: "No migrations found. Run from project root: migrate schema, then import Cricsheet JSON.",
			Command:     "make migrate && make cricsheet-import",
			Priority:    "HIGH",
		}}
	}

	if lastRetrain == nil || lastRetrain.StartedAt.Before(lastImport.StartedAt) {
		return []Suggestion{{
			Title: "Retrain",
			Description: "New data imported, so the ratings and the models are behind it. Retrain runs the " +
				"rating pass, the XI win models, the performance models and L4's report into a new run.",
			Command:  "make retrain CUTOFF=2025-09-01",
			Priority: "HIGH",
		}}
	}

	if lastReload == nil || lastReload.StartedAt.Before(lastRetrain.StartedAt) {
		return []Suggestion{{
			Title: "Reload",
			Description: "A run has been trained but nothing is serving it: reload points `current` at a run " +
				"and loads it into the running service.",
			Command:  "make reload",
			Priority: "HIGH",
		}}
	}

	return []Suggestion{}
}
