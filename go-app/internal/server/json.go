package server

import (
	"encoding/json"
	"log/slog"
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

// respondErr writes a standardized 500 Internal Server Error response using apiError and logs the error.
func respondErr(w http.ResponseWriter, err error) {
	if err != nil {
		slog.Error("handler returning 500 Internal Server Error", slog.Any("err", err))
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	writeJSON(w, http.StatusInternalServerError, apiError{Code: "INTERNAL", Message: msg})
}

// respondBadRequest writes a standardized 400 Bad Request response using apiError and logs at info level when err is set.
func respondBadRequest(w http.ResponseWriter, err error) {
	msg := ""
	if err != nil {
		slog.Info("handler returning 400 Bad Request", slog.Any("err", err))
		msg = err.Error()
	}
	writeJSON(w, http.StatusBadRequest, apiError{Code: "BAD_REQUEST", Message: msg})
}
