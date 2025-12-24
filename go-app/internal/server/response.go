// revive:disable:var-naming
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// respondJSON writes a JSON response with the provided status code and value.
func respondJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("respondJSON encode failed", slog.Any("err", err))
	}
}

// respondErr writes a 500 error with the error message in JSON.
func respondErr(w http.ResponseWriter, err error) {
	respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

// respondBadRequest writes a 400 error with the error message in JSON.
func respondBadRequest(w http.ResponseWriter, err error) {
	respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}
