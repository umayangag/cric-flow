// revive:disable:var-naming
package server

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
	r.HandleFunc("/import/cricsheet", importCricSheetHandler).Methods(http.MethodPost)

	// Domain queries
	r.HandleFunc("/players/{id}", getPlayerHandler).Methods(http.MethodGet)
	r.HandleFunc("/matches/{id}", getMatchHandler).Methods(http.MethodGet)

	// ML predictions
	r.HandleFunc("/predict/batting", a.predictBattingHandler).Methods(http.MethodPost)
	r.HandleFunc("/predict/bowling", a.predictBowlingHandler).Methods(http.MethodPost)

	// Seasons/Matches for DB-backed evaluation
	r.HandleFunc("/seasons/next", getNextSeasonHandler).Methods(http.MethodGet)
	r.HandleFunc("/matches", listMatchesHandler).Methods(http.MethodGet)

	// Match squads for DB-backed evaluation/compare
	r.HandleFunc("/match/{id}/squads", getMatchSquadsHandler).Methods(http.MethodGet)

	// Wrap with CORS middleware for frontend access
	return corsMiddleware(r)
}
