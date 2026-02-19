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
	admin.HandleFunc("/precompute", a.precomputeHandler).Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/precompute/status", precomputeStatusHandler).Methods(http.MethodGet, http.MethodOptions)

	// Import cricsheet data
	admin.HandleFunc("/import/cricsheet", a.importCricSheetHandler).Methods(http.MethodPost, http.MethodOptions)

	// Ops status aggregator (observability)
	admin.HandleFunc("/ops/status", a.opsStatusHandler).Methods(http.MethodGet, http.MethodOptions)

	// Ops Migrations
	opsHandler := &OpsHandler{}
	admin.HandleFunc("/ops/migrations", opsHandler.ListMigrations).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/ops/suggestions", opsHandler.GetSuggestions).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/ops/pipeline/run/{step}", a.pipelineRunHandler).Methods(http.MethodPost, http.MethodOptions)

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
	// Future match team selection: best 11 for each team
	admin.HandleFunc("/api/predict/team-selection", a.predictTeamSelectionHandler).
		Methods(http.MethodPost, http.MethodGet, http.MethodOptions)

	// Backtesting endpoints
	admin.HandleFunc("/api/backtest/match", a.backtestMatchHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/backtest/evaluate-stream", a.backtestEvaluateStreamHandler).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/backtest/evaluate-start", a.backtestEvaluateStartHandler).
		Methods(http.MethodPost, http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/backtest/evaluate-status", a.backtestEvaluateStatusHandler).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/backtest/scorecard", a.backtestScorecardHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/backtest/training-data", a.backtestTrainingDataHandler).
		Methods(http.MethodGet, http.MethodOptions)
	// Walk-forward: list matches after a cutoff (for chunking)
	admin.HandleFunc("/api/backtest/matches", a.backtestMatchesHandler).
		Methods(http.MethodGet, http.MethodOptions)
	// Walk-forward: holdout data (features at cutoff for matches after cutoff)
	admin.HandleFunc("/api/backtest/holdout-data", a.backtestHoldoutDataHandler).
		Methods(http.MethodGet, http.MethodOptions)
	// Accuracy trend endpoint for dashboards
	admin.HandleFunc("/api/backtest/accuracy-trend", a.backtestAccuracyTrendHandler).
		Methods(http.MethodGet, http.MethodOptions)

	// ML tuned params: save/retrieve auto-tuned training params per model (and format)
	admin.HandleFunc("/api/ml/tuned-params", a.mlTunedParamsGetHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/ml/tuned-params", a.mlTunedParamsPostHandler).Methods(http.MethodPost, http.MethodOptions)

	// Legacy evaluatedb routes removed: /seasons/next, /matches, /match/{id}/squads
	// The new backtesting flow is exposed via /api/backtest/match (select and evaluate modes).

	// Wrap with CORS middleware for frontend access
	return corsMiddleware(r)
}
