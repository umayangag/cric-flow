package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

// NewRouter constructs and returns the API HTTP router with all routes registered.
func NewRouter(a *App) http.Handler {
	r := mux.NewRouter()

	// Liveness
	r.HandleFunc("/health", healthHandler).Methods(http.MethodGet)

	// Readiness (checks DB connectivity)
	r.HandleFunc("/readiness", readinessHandler).Methods(http.MethodGet)

	// Precompute controls
	r.HandleFunc("/precompute", precomputeHandler).Methods(http.MethodPost)
	r.HandleFunc("/precompute/status", precomputeStatusHandler).Methods(http.MethodGet)

	// Import cricsheet data
	r.HandleFunc("/import/cricsheet", importCricsheetHandler).Methods(http.MethodPost)

	// Domain queries
	r.HandleFunc("/players/{id}", getPlayerHandler).Methods(http.MethodGet)
	r.HandleFunc("/matches/{id}", getMatchHandler).Methods(http.MethodGet)

	// ML predictions
	r.HandleFunc("/predict/batting", a.predictBattingHandler).Methods(http.MethodPost)
	r.HandleFunc("/predict/bowling", a.predictBowlingHandler).Methods(http.MethodPost)

	return r
}
