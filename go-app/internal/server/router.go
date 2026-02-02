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

	// Backtesting endpoints
	r.HandleFunc("/api/backtest/match", a.backtestMatchHandler).Methods(http.MethodGet)
	// Accuracy trend endpoint for dashboards
	r.HandleFunc("/api/backtest/accuracy-trend", a.backtestAccuracyTrendHandler).Methods(http.MethodGet)

	// Ops status aggregator (observability)
	r.HandleFunc("/ops/status", a.opsStatusHandler).Methods(http.MethodGet)

	// Legacy evaluatedb routes removed: /seasons/next, /matches, /match/{id}/squads
	// The new backtesting flow is exposed via /api/backtest/match (select and evaluate modes).

	// Wrap with CORS middleware for frontend access
	return corsMiddleware(r)
}
