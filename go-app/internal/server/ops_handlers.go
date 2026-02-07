package server

import (
	"net/http"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
)

type OpsHandler struct{}

func (h *OpsHandler) ListMigrations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	migrations, err := tracking.GetRecentMigrations(ctx, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, migrations)
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

	var suggestions []Suggestion

	// Logic to derive suggestions
	// Find last successful run of each type
	var lastImport, lastPrecompute, lastExport, lastTrainBatting, lastTrainBowling *tracking.Migration

	for i := range migrations {
		m := &migrations[i]
		if m.Status != tracking.StatusCompleted {
			continue
		}
		if m.Command == "cricsheet-import" && lastImport == nil {
			lastImport = m
		}
		if m.Command == "precompute-features" && lastPrecompute == nil {
			lastPrecompute = m
		}
		if m.Command == "export-dataset" && lastExport == nil {
			lastExport = m
		}
		if m.Command == "train-batting" && lastTrainBatting == nil {
			lastTrainBatting = m
		}
		if m.Command == "train-bowling" && lastTrainBowling == nil {
			lastTrainBowling = m
		}
	}

	// Rule 1: Import -> Precompute
	if lastImport != nil {
		if lastPrecompute == nil || lastPrecompute.StartedAt.Before(lastImport.StartedAt) {
			suggestions = append(suggestions, Suggestion{
				Title:       "Run Precompute",
				Description: "New data imported. Run precompute features to update feature store.",
				Command:     "make precompute-asof",
				Priority:    "HIGH",
			})
		}
	}

	// Rule 2: Precompute -> Export
	if lastPrecompute != nil {
		if lastExport == nil || lastExport.StartedAt.Before(lastPrecompute.StartedAt) {
			suggestions = append(suggestions, Suggestion{
				Title:       "Export Dataset",
				Description: "Features updated. Export dataset for training.",
				Command:     "make export-dataset",
				Priority:    "MEDIUM",
			})
		}
	}

	// Rule 3: Export -> Train
	if lastExport != nil {
		// Batting
		if lastTrainBatting == nil || lastTrainBatting.StartedAt.Before(lastExport.StartedAt) {
			suggestions = append(suggestions, Suggestion{
				Title:       "Train Batting Model",
				Description: "New dataset exported. Train the batting model.",
				Command:     "make train-batting",
				Priority:    "MEDIUM",
			})
		}
		// Bowling
		if lastTrainBowling == nil || lastTrainBowling.StartedAt.Before(lastExport.StartedAt) {
			suggestions = append(suggestions, Suggestion{
				Title:       "Train Bowling Model",
				Description: "New dataset exported. Train the bowling model.",
				Command:     "make train-bowling",
				Priority:    "MEDIUM",
			})
		}
	}

	// Fallback if nothing ran
	if len(migrations) == 0 {
		suggestions = append(suggestions, Suggestion{
			Title:       "Initialize Data",
			Description: "No migrations found. Start by importing data.",
			Command:     "make cricsheet-import",
			Priority:    "HIGH",
		})
	}

	if suggestions == nil {
		suggestions = []Suggestion{}
	}

	writeJSON(w, http.StatusOK, suggestions)
}
