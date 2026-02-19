package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// POST /api/ml/tuned-params body: { "model": "batting", "format": "T20", "params": { ... } }
func (a *App) mlTunedParamsPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST required"})
		return
	}
	var body struct {
		Model  string          `json:"model"`
		Format string          `json:"format"`
		Params json.RawMessage `json:"params"`
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
	if err := db.InsertMLTunedParams(r.Context(), model, format, body.Params); err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "model": model, "format": format})
}

// GET /api/ml/tuned-params?model=batting&format=T20 returns latest { "params": {...}, "created_at": "..." } or 404
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
		writeJSON(w, http.StatusNotFound, apiError{Code: "NOT_FOUND", Message: "no tuned params for this model and format"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"params":     json.RawMessage(row.Params),
		"created_at": row.CreatedAt,
	})
}
