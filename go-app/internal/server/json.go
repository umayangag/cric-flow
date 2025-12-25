package server

import (
	"encoding/json"
	"net/http"
)

// apiError is the standard error payload for API responses.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// writeJSON writes a pretty-printed JSON response with the provided status code and value.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// respondErr writes a standardized 500 Internal Server Error response using apiError.
func respondErr(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, apiError{Code: "INTERNAL", Message: err.Error()})
}

// respondBadRequest writes a standardized 400 Bad Request response using apiError.
func respondBadRequest(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, apiError{Code: "BAD_REQUEST", Message: err.Error()})
}
