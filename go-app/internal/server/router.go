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

	// Admin/Ops routes (protected by auth)
	admin := r.PathPrefix("").Subrouter()
	admin.Use(authMiddleware)

	// Precompute controls
	admin.HandleFunc("/precompute", precomputeHandler).Methods(http.MethodPost)
	admin.HandleFunc("/precompute/status", precomputeStatusHandler).Methods(http.MethodGet)

	// Import cricsheet data
	admin.HandleFunc("/import/cricsheet", importCricSheetHandler).Methods(http.MethodPost)

	// Ops status aggregator (observability)
	admin.HandleFunc("/ops/status", a.opsStatusHandler).Methods(http.MethodGet)

	// Ops Migrations
	opsHandler := &OpsHandler{}
	admin.HandleFunc("/ops/migrations", opsHandler.ListMigrations).Methods(http.MethodGet)
	admin.HandleFunc("/ops/suggestions", opsHandler.GetSuggestions).Methods(http.MethodGet)

	// Options
	optionsHandler := &OptionsHandler{}
	admin.HandleFunc("/api/options/teams", optionsHandler.HandleGetTeams).Methods(http.MethodGet)
	admin.HandleFunc("/api/options/formats", optionsHandler.HandleGetFormats).Methods(http.MethodGet)

	// Domain queries
	admin.HandleFunc("/players/{id}", getPlayerHandler).Methods(http.MethodGet)
	admin.HandleFunc("/matches/{id}", getMatchHandler).Methods(http.MethodGet)

	// ML predictions
	admin.HandleFunc("/predict/batting", a.predictBattingHandler).Methods(http.MethodPost)
	admin.HandleFunc("/predict/bowling", a.predictBowlingHandler).Methods(http.MethodPost)

	// Backtesting endpoints
	admin.HandleFunc("/api/backtest/match", a.backtestMatchHandler).Methods(http.MethodGet)
	// Accuracy trend endpoint for dashboards
	admin.HandleFunc("/api/backtest/accuracy-trend", a.backtestAccuracyTrendHandler).Methods(http.MethodGet)

	// Legacy evaluatedb routes removed: /seasons/next, /matches, /match/{id}/squads
	// The new backtesting flow is exposed via /api/backtest/match (select and evaluate modes).

	// Wrap with CORS middleware for frontend access
	return corsMiddleware(r)
}
