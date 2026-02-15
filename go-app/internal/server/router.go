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
	admin := r.NewRoute().Subrouter()
	admin.Use(authMiddleware)

	// Precompute controls
	admin.HandleFunc("/precompute", precomputeHandler).Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/precompute/status", precomputeStatusHandler).Methods(http.MethodGet, http.MethodOptions)

	// Import cricsheet data
	admin.HandleFunc("/import/cricsheet", importCricSheetHandler).Methods(http.MethodPost, http.MethodOptions)

	// Ops status aggregator (observability)
	admin.HandleFunc("/ops/status", a.opsStatusHandler).Methods(http.MethodGet, http.MethodOptions)

	// Ops Migrations
	opsHandler := &OpsHandler{}
	admin.HandleFunc("/ops/migrations", opsHandler.ListMigrations).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/ops/suggestions", opsHandler.GetSuggestions).Methods(http.MethodGet, http.MethodOptions)

	// Options
	optionsHandler := &OptionsHandler{}
	admin.HandleFunc("/api/options/teams", optionsHandler.HandleGetTeams).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/options/teams-by-format", optionsHandler.HandleGetTeamsByFormat).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/options/opponents", optionsHandler.HandleGetOpponents).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/options/formats", optionsHandler.HandleGetFormats).
		Methods(http.MethodGet, http.MethodOptions)

	// Domain queries
	admin.HandleFunc("/players/{id}", getPlayerHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/matches/{id}", getMatchHandler).Methods(http.MethodGet, http.MethodOptions)

	// ML predictions
	admin.HandleFunc("/predict/batting", a.predictBattingHandler).Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/predict/bowling", a.predictBowlingHandler).Methods(http.MethodPost, http.MethodOptions)

	// Backtesting endpoints
	admin.HandleFunc("/api/backtest/match", a.backtestMatchHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/backtest/evaluate-stream", a.backtestEvaluateStreamHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/backtest/scorecard", a.backtestScorecardHandler).Methods(http.MethodGet, http.MethodOptions)
	// Accuracy trend endpoint for dashboards
	admin.HandleFunc("/api/backtest/accuracy-trend", a.backtestAccuracyTrendHandler).
		Methods(http.MethodGet, http.MethodOptions)

	// Legacy evaluatedb routes removed: /seasons/next, /matches, /match/{id}/squads
	// The new backtesting flow is exposed via /api/backtest/match (select and evaluate modes).

	// Wrap with CORS middleware for frontend access
	return corsMiddleware(r)
}
