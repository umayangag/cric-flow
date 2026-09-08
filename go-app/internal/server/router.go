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
	admin.HandleFunc("/api/health/ml", a.mlServiceProxy("/health", "ml health proxy")).
		Methods(http.MethodGet, http.MethodOptions)
	// Which run is loaded, how far its ratings go, and whether they are stale (H-11, H-16).
	admin.HandleFunc("/api/ml/xi-status", a.mlServiceProxy("/xi/status", "xi status proxy")).
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
	// The state of the acquired player biographies (X-1a): how much of the archive they
	// cover, per format and gender, and how stale the acquisition is. It is a data
	// source's state, so it belongs beside the dataset registry rather than in a log.
	admin.HandleFunc("/ops/data/biography-coverage", a.dataBiographyCoverageHandler).
		Methods(http.MethodGet, http.MethodOptions)

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
	// The candidate list a user picks a pool out of, and what the ledger is keeping out
	// of it (D-12). It is an options route because that is what it is: the choices a
	// prediction request can be built from.
	admin.HandleFunc("/api/options/candidates", a.candidatesHandler).
		Methods(http.MethodGet, http.MethodOptions)

	// The cross-club player search (P3-1). It is not an options route: the candidate
	// list above is per club because a prediction is about one side, and an auction room
	// is not one side. Registered before /players/{id} so a search is not read as a
	// lookup of a player called "search".
	admin.HandleFunc("/api/players/search", a.searchPlayersHandler).
		Methods(http.MethodGet, http.MethodOptions)

	// Domain queries
	admin.HandleFunc("/players/{id}", getPlayerHandler).Methods(http.MethodGet, http.MethodOptions)
	// The retirement ledger: a user's claim that a player has retired, and its
	// withdrawal. POST records the claim and promotes it to the stored fact only where a
	// criterion corroborates it; DELETE withdraws the claim and demotes the fact it
	// raised (D-12).
	admin.HandleFunc("/api/players/{id}/retirement", a.retirementFlagHandler).
		Methods(http.MethodPost, http.MethodDelete, http.MethodOptions)
	admin.HandleFunc("/matches/{id}", getMatchHandler).Methods(http.MethodGet, http.MethodOptions)

	// Future match prediction: both XIs, the win probability and the simulated scorecard.
	admin.HandleFunc("/api/predict/team-selection", a.predictTeamSelectionHandler).
		Methods(http.MethodPost, http.MethodGet, http.MethodOptions)

	// The prediction record (P2-3): every answer this API has issued, as it was served.
	// It is a read surface only — the one writer is the prediction endpoint above, which
	// files an answer on its way out. The listing is registered before the by-id route so
	// gorilla/mux matches the bare path against it rather than treating an empty id as a
	// lookup.
	admin.HandleFunc("/api/predictions", a.listPredictionsHandler).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/predictions/{id}", a.getPredictionHandler).
		Methods(http.MethodGet, http.MethodOptions)
	// The track record (P2-4): the record above scored against the match tables on
	// every read -- no column, no step, no scheduler. Misses included.
	admin.HandleFunc("/api/track-record", a.trackRecordHandler).
		Methods(http.MethodGet, http.MethodOptions)

	// The auction record (P3-1): facts the operator enters as a live auction runs, and
	// the remaining pool's distribution by the model's two role predicates. Every write
	// answers with the auction as it now stands, because that is what the next decision
	// is made against. Nothing on these routes reaches /xi/optimize, and nothing on them
	// returns a win probability or a marginal value: in T20 this system has not shown it
	// can choose an eleven better than rating order (plan §8.8), and the module does not
	// try to. The listing is registered before the by-id route so gorilla/mux matches the
	// bare path against it rather than treating an empty id as a lookup.
	admin.HandleFunc("/api/auctions", a.listAuctionsHandler).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/auctions", a.createAuctionHandler).
		Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/api/auctions/{id}", a.getAuctionHandler).
		Methods(http.MethodGet, http.MethodOptions)
	admin.HandleFunc("/api/auctions/{id}/players", a.addAuctionPlayersHandler).
		Methods(http.MethodPost, http.MethodOptions)
	admin.HandleFunc("/api/auctions/{id}/outcomes", a.recordAuctionOutcomeHandler).
		Methods(http.MethodPost, http.MethodOptions)

	// Backtesting: L4's evaluation report is the whole surface. The per-match evaluate
	// flow scored the batting / bowling / fielding models and went with them (P-5).
	admin.HandleFunc("/api/backtest/report", a.mlServiceProxy("/xi/evaluate-report", "xi evaluate report proxy")).
		Methods(http.MethodGet, http.MethodOptions)
	// The metric glossary (L-1) is served from ml-service's code, not from a report on
	// disk: every surface that shows a number renders it, including those that never read
	// the evaluation report, and none of them holds metric prose of its own.
	admin.HandleFunc("/api/backtest/metric-glossary", a.mlServiceProxy("/xi/metric-glossary", "xi metric glossary proxy")).
		Methods(http.MethodGet, http.MethodOptions)
	// Wrap with CORS middleware for frontend access
	return corsMiddleware(r)
}
