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
	// Retired parameters are refused here, once, rather than in each handler that
	// used to read them. See removed_params.go.
	admin.Use(rejectRemovedParams)

	// Import cricsheet data
	admin.HandleFunc("/import/cricsheet", a.importCricSheetHandler).Methods(http.MethodPost, http.MethodOptions)

	// ML health proxy (full response for Health tab: loaded formats, artifacts)
	admin.HandleFunc("/api/health/ml", a.mlServiceProxy("/health", "ml health proxy", nil)).
		Methods(http.MethodGet, http.MethodOptions)
	// Which run is loaded, how far its ratings go, and whether they are stale (H-11, H-16).
	admin.HandleFunc("/api/ml/xi-status", a.mlServiceProxy("/xi/status", "xi status proxy", nil)).
		Methods(http.MethodGet, http.MethodOptions)
	// Ops status aggregator (observability)
	admin.HandleFunc("/ops/status", a.opsStatusHandler).Methods(http.MethodGet, http.MethodOptions)

	// Ops Migrations
	opsHandler := &OpsHandler{}
	admin.HandleFunc("/ops/migrations", opsHandler.ListMigrations).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/ops/suggestions", opsHandler.GetSuggestions).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/ops/pipeline/run/{step}", a.pipelineRunHandler).Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/ops/pipeline/stop", a.pipelineStopHandler).Methods(http.MethodPost, http.MethodOptions)
	// Run plans: the API equivalent of `make up-all` / `make full-pipeline`.
	admin.HandleFunc("/ops/pipeline/run-plan", a.runPlanStartHandler).Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/ops/pipeline/plan", a.runPlanStateHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/ops/pipeline/stream", a.pipelineProgressStreamHandler).
		Methods(http.MethodGet, http.MethodOptions)

	// Dataset acquisition. These live under /ops/data rather than
	// /ops/pipeline/run/{step} because acquisition is what you do before the
	// pipeline, not a stage of it — the same distinction Step.Surface encodes.
	admin.HandleFunc("/ops/data/feeds", a.dataFeedsHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/ops/data/fetch", a.dataFetchHandler).Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/ops/data/staged", a.dataStagedHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/ops/data/extract", a.dataExtractHandler).Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/ops/data/datasets", a.dataDatasetsHandler).Methods(http.MethodGet, http.MethodOptions)

	// Options
	optionsHandler := &OptionsHandler{}
	// Team options are *sides*, not names: each carries the club id a prediction request
	// must send, because a name alone names two teams for a third of the dataset (D-10).
	admin.HandleFunc("/api/options/teams-by-format", optionsHandler.HandleGetTeamSidesByFormat).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/options/opponents", optionsHandler.HandleGetOpponentSides).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/options/formats", optionsHandler.HandleGetFormats).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/canonical/formats", optionsHandler.HandleGetCanonicalFormats).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/options/venues", optionsHandler.HandleGetVenues).
		Methods(http.MethodGet, http.MethodOptions)

	// Domain queries
	admin.HandleFunc("/players/{id}", getPlayerHandler).Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/matches/{id}", getMatchHandler).Methods(http.MethodGet, http.MethodOptions)

	// Future match prediction: both XIs, the win probability and the simulated scorecard.
	admin.HandleFunc("/api/predict/team-selection", a.predictTeamSelectionHandler).
		Methods(http.MethodPost, http.MethodGet, http.MethodOptions)

	// Backtesting: L4's evaluation report is the whole surface. The per-match evaluate
	// flow scored the batting / bowling / fielding models and went with them (P-5).
	admin.HandleFunc("/api/backtest/report", a.mlServiceProxy("/xi/evaluate-report", "xi evaluate report proxy", nil)).
		Methods(http.MethodGet, http.MethodOptions)
	// Wrap with CORS middleware for frontend access
	return corsMiddleware(r)
}
