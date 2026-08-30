package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
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

// GetAutoTuneDetails returns details of tuned params linked to a specific data migration.
// Route: GET /ops/migrations/{id}/auto-tune
func (h *OpsHandler) GetAutoTuneDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	migrationID, err := strconv.Atoi(idStr)
	if err != nil || migrationID <= 0 {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid migration id"})
		return
	}

	rows, err := db.ListMLTunedParamsByMigration(r.Context(), migrationID)
	if err != nil {
		slog.Error(
			"ops: ListMLTunedParamsByMigration failed",
			slog.Int("migration_id", migrationID),
			slog.Any("err", err),
		)
		respondErr(w, err)
		return
	}

	type responseRun struct {
		ID        int             `json:"id"`
		Model     string          `json:"model"`
		Format    string          `json:"format"`
		CreatedAt string          `json:"created_at"`
		Params    json.RawMessage `json:"params,omitempty"`
		Metrics   json.RawMessage `json:"metrics,omitempty"`
	}

	resp := struct {
		MigrationID int           `json:"migration_id"`
		Runs        []responseRun `json:"runs"`
	}{
		MigrationID: migrationID,
		Runs:        make([]responseRun, 0, len(rows)),
	}

	for _, row := range rows {
		var paramsRaw, metricsRaw json.RawMessage
		if len(row.Params) > 0 {
			paramsRaw = row.Params
		}
		if len(row.Metrics) > 0 {
			metricsRaw = row.Metrics
		}

		resp.Runs = append(resp.Runs, responseRun{
			ID:        row.ID,
			Model:     row.Model,
			Format:    row.Format,
			CreatedAt: row.CreatedAt,
			Params:    paramsRaw,
			Metrics:   metricsRaw,
		})
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
			Title:       "Fix Missing Features",
			Description: "Feature tables (form, consistency, sequence) are empty or incomplete. Run from project root to compute all features for all formats.",
			Command:     "make precompute-all-all-formats",
			Priority:    "HIGH",
		}}
	}

	// Rule 1: Import -> Precompute
	if lastPrecompute == nil || lastPrecompute.StartedAt.Before(lastImport.StartedAt) {
		return []Suggestion{{
			Title:       "Run Full Precompute",
			Description: "New data imported. Run from project root to compute all features (form, consistency, sequence) for all formats (TEST, ODI, T20, T20I).",
			Command:     "make precompute-all-all-formats",
			Priority:    "HIGH",
		}}
	}

	// Rule 2: Precompute -> Export
	if lastExport == nil || lastExport.StartedAt.Before(lastPrecompute.StartedAt) {
		return []Suggestion{{
			Title:       "Export Dataset",
			Description: "Features updated. Exports the cross-format and per-format CSVs to output/go-app.",
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
			Description: "New dataset exported. Run from project root. Produces one model per format. Run make ml-install first if venv deps are missing.",
			Command:     "make train-batting",
			Priority:    "MEDIUM",
		})
	}
	// Bowling
	if lastTrainBowling == nil || lastTrainBowling.StartedAt.Before(lastExport.StartedAt) {
		trainSuggestions = append(trainSuggestions, Suggestion{
			Title:       "Train Bowling Model",
			Description: "New dataset exported. Run from project root. Produces one model per format. Run make ml-install first if venv deps are missing.",
			Command:     "make train-bowling",
			Priority:    "MEDIUM",
		})
	}

	if len(trainSuggestions) > 0 {
		return trainSuggestions
	}

	return []Suggestion{}
}
