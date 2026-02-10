package server

import (
	"net/http"
	"strconv"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
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

	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	migrations, total, err := tracking.GetMigrationsPaginated(ctx, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"items": migrations,
		"total": total,
		"page":  page,
		"limit": limit,
	}

	writeJSON(w, http.StatusOK, response)
}

type Suggestion struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command"`
	Priority    string `json:"priority"` // HIGH, MEDIUM, LOW
}

func (h *OpsHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	migrations, err := tracking.GetRecentMigrations(ctx, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	suggestions := GenerateSuggestions(migrations)

	writeJSON(w, http.StatusOK, suggestions)
}

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
	lastPrecompute := lastRuns["precompute-features"]
	lastExport := lastRuns["export-dataset"]
	lastTrainBatting := lastRuns["train-batting"]
	lastTrainBowling := lastRuns["train-bowling"]

	// Rule 0: Initialize if no successful import found
	if lastImport == nil {
		return []Suggestion{{
			Title:       "Initialize Data",
			Description: "No migrations found. Start by importing data.",
			Command:     "make cricsheet-import",
			Priority:    "HIGH",
		}}
	}

	// Rule 1: Import -> Precompute
	if lastPrecompute == nil || lastPrecompute.StartedAt.Before(lastImport.StartedAt) {
		return []Suggestion{{
			Title:       "Run Precompute",
			Description: "New data imported. Run precompute features to update feature store.",
			Command:     "make precompute-asof",
			Priority:    "HIGH",
		}}
	}

	// Rule 2: Precompute -> Export
	if lastExport == nil || lastExport.StartedAt.Before(lastPrecompute.StartedAt) {
		return []Suggestion{{
			Title:       "Export Dataset",
			Description: "Features updated. Export dataset for training.",
			Command:     "make export-dataset",
			Priority:    "MEDIUM",
		}}
	}

	// Rule 3: Export -> Train
	var trainSuggestions []Suggestion
	// Batting
	if lastTrainBatting == nil || lastTrainBatting.StartedAt.Before(lastExport.StartedAt) {
		trainSuggestions = append(trainSuggestions, Suggestion{
			Title:       "Train Batting Model",
			Description: "New dataset exported. Train the batting model.",
			Command:     "make train-batting",
			Priority:    "MEDIUM",
		})
	}
	// Bowling
	if lastTrainBowling == nil || lastTrainBowling.StartedAt.Before(lastExport.StartedAt) {
		trainSuggestions = append(trainSuggestions, Suggestion{
			Title:       "Train Bowling Model",
			Description: "New dataset exported. Train the bowling model.",
			Command:     "make train-bowling",
			Priority:    "MEDIUM",
		})
	}

	if len(trainSuggestions) > 0 {
		return trainSuggestions
	}

	return []Suggestion{}
}
