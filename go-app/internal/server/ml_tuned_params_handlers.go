package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// POST /api/ml/tuned-params body: { "model": "batting", "format": "T20", "params": { ... }, "metrics": { ... } }
func (a *App) mlTunedParamsPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST required"})
		return
	}
	var body struct {
		Model   string          `json:"model"`
		Format  string          `json:"format"`
		Params  json.RawMessage `json:"params"`
		Metrics json.RawMessage `json:"metrics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_JSON", Message: err.Error()})
		return
	}
	model := strings.TrimSpace(strings.ToLower(body.Model))
	if model == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "model is required"})
		return
	}
	format := strings.TrimSpace(strings.ToUpper(body.Format))
	if body.Params == nil {
		body.Params = []byte("{}")
	}
	dataMigrationID, migrationErr := tracking.GetInProgressMigrationIDForCommand(r.Context(), "ml-auto-tune")
	if migrationErr != nil {
		slog.Warn("failed to get in-progress migration ID for ml-auto-tune", "err", migrationErr)
	}
	if err := db.InsertMLTunedParams(r.Context(), model, format, body.Params, body.Metrics, dataMigrationID); err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "model": model, "format": format})
}

// GET /api/ml/tuned-params/list returns all stored (model, format) with latest created_at for each.
func (a *App) mlTunedParamsListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "GET required"})
		return
	}
	entries, err := db.ListLatestMLTunedParams(r.Context())
	if err != nil {
		respondErr(w, err)
		return
	}
	// Return as list of objects so clients see which model and format each params row belongs to.
	list := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		m := map[string]interface{}{"model": e.Model, "format": e.Format, "created_at": e.CreatedAt}
		if len(e.Metrics) > 0 {
			m["metrics"] = json.RawMessage(e.Metrics)
		}
		list = append(list, m)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"entries": list})
}

// GET /api/ml/tuned-params?model=batting&format=T20 returns latest { "model", "format", "params", "created_at" } or 404
func (a *App) mlTunedParamsGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "GET required"})
		return
	}
	model := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("model")))
	if model == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "model is required"})
		return
	}
	format := strings.TrimSpace(strings.ToUpper(r.URL.Query().Get("format")))
	row, err := db.GetLatestMLTunedParams(r.Context(), model, format)
	if err != nil {
		respondErr(w, err)
		return
	}
	if row == nil {
		writeJSON(
			w,
			http.StatusNotFound,
			apiError{Code: "NOT_FOUND", Message: "no tuned params for this model and format"},
		)
		return
	}
	// Include model and format so the response is self-describing (which model/format the params belong to).
	out := map[string]interface{}{
		"model":      model,
		"format":     format,
		"params":     json.RawMessage(row.Params),
		"created_at": row.CreatedAt,
	}
	if len(row.Metrics) > 0 {
		out["metrics"] = json.RawMessage(row.Metrics)
	}
	writeJSON(w, http.StatusOK, out)
}
