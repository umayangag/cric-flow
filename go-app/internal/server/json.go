package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// apiError is the standard error payload for API responses.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	// Available lists what the caller could have asked for, when the failure is that
	// the thing they named was not there. For a model that is not loaded this is the
	// whole answer, so it is part of the error rather than something the client has to
	// go and look up.
	Available []string `json:"available,omitempty"`
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
	// A failure that already knows what it is keeps its own status, code and hint.
	// Collapsing an upstream 404 "no T20I model loaded; you have ODI and TEST" into a
	// 500 INTERNAL with the whole thing concatenated into one string is how a
	// perfectly actionable error reached the UI as an unexplained crash (W1-2).
	var mlErr *mlServiceError
	if errors.As(err, &mlErr) {
		slog.Info("relaying ml-service error",
			slog.String("endpoint", mlErr.Endpoint),
			slog.Int("status", mlErr.Status),
			slog.String("code", mlErr.Code))
		writeJSON(w, relayStatus(mlErr.Status), apiError{
			Code:      mlErr.Code,
			Message:   mlErr.Message,
			Hint:      mlErr.Hint,
			Available: mlErr.Available,
		})
		return
	}

	if err != nil {
		slog.Error("handler returning 500 Internal Server Error", slog.Any("err", err))
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	writeJSON(w, http.StatusInternalServerError, apiError{Code: "INTERNAL", Message: msg})
}

// relayStatus maps an upstream status onto one this API may answer with.
//
// 4xx passes through: "you asked for a format with no model" is the caller's problem
// whichever service noticed it. Anything else becomes 502 — ml-service failing is this
// service's dependency failing, not this request being malformed, and saying 500 would
// blame the wrong side.
func relayStatus(upstream int) int {
	if upstream >= 400 && upstream < 500 {
		return upstream
	}
	return http.StatusBadGateway
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
