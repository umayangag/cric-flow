package server

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
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

	// Prevent extremely large page values which can cause expensive OFFSET operations
	if page > 10000 {
		slog.Warn("pagination page capped", "requested", page, "capped_at", 10000)
		page = 10000
	}

	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	// Enforce an upper bound to prevent resource exhaustion
	if limit > 100 {
		slog.Warn("pagination limit capped", "requested", limit, "capped_at", 100)
		limit = 100
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
	migrations, err := tracking.GetRecentMigrations(ctx, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	seqPopulated, err := db.IsSequenceFeaturesPopulated(ctx)
	if err != nil {
		slog.Error("ops: IsSequenceFeaturesPopulated check failed", slog.Any("err", err))
		seqPopulated = false
	}

	suggestions := GenerateSuggestions(migrations, seqPopulated)

	writeJSON(w, http.StatusOK, suggestions)
}

func GenerateSuggestions(migrations []tracking.Migration, seqPopulated bool) []Suggestion {
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
			Description: "No migrations found. Run from project root: migrate schema, then import Cricsheet JSON.",
			Command:     "make migrate && make cricsheet-import",
			Priority:    "HIGH",
		}}
	}

	// Rule: Missing Sequence Data (Critical fix)
	// Even if precompute is recent, if data is missing, we must re-run.
	if !seqPopulated {
		return []Suggestion{{
			Title:       "Fix Missing Sequence Features",
			Description: "Sequence feature tables are empty. Run from project root. Uses today's date; add ASOF=YYYY-MM-DD for a specific date.",
			Command:     "make precompute-asof",
			Priority:    "HIGH",
		}}
	}

	// Rule 1: Import -> Precompute
	if lastPrecompute == nil || lastPrecompute.StartedAt.Before(lastImport.StartedAt) {
		return []Suggestion{{
			Title:       "Run Precompute",
			Description: "New data imported. Run from project root. Uses today's date; add ASOF=YYYY-MM-DD for a specific date.",
			Command:     "make precompute-asof",
			Priority:    "HIGH",
		}}
	}

	// Rule 2: Precompute -> Export
	if lastExport == nil || lastExport.StartedAt.Before(lastPrecompute.StartedAt) {
		return []Suggestion{{
			Title:       "Export Dataset",
			Description: "Features updated. Run from project root. Exports unified cross-format CSVs to output/go-app.",
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
			Description: "New dataset exported. Run from project root. Run make ml-install first if venv deps are missing.",
			Command:     "make train-batting",
			Priority:    "MEDIUM",
		})
	}
	// Bowling
	if lastTrainBowling == nil || lastTrainBowling.StartedAt.Before(lastExport.StartedAt) {
		trainSuggestions = append(trainSuggestions, Suggestion{
			Title:       "Train Bowling Model",
			Description: "New dataset exported. Run from project root. Run make ml-install first if venv deps are missing.",
			Command:     "make train-bowling",
			Priority:    "MEDIUM",
		})
	}

	if len(trainSuggestions) > 0 {
		return trainSuggestions
	}

	return []Suggestion{}
}
