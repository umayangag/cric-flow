package server

import (
	"log/slog"
	"net/http"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
	"github.com/umayangag/cric-flow/go-app/internal/services/runplan"
)

// startImportPlan runs fetch → extract → import as one plan and reports whether it
// took the request.
//
// It answers false — leaving the caller to import the directory as it stands — only
// when acquisition is impossible or already in flight. Falling back to a bare import
// in those cases is deliberate: an operator whose box has no outbound network should
// still be able to import data they put there by hand, and a plan already running is
// not a reason to refuse the work, it is a reason not to start a second one.
func (a *App) startImportPlan(w http.ResponseWriter, r *http.Request, body cricSheetRequest) bool {
	steps, err := runplan.Resolve(runplan.PlanImport, nil)
	if err != nil {
		// Only reachable if the plan is removed from the registry, which the plan
		// test would catch first. Import still works; acquisition does not.
		slog.Error("import: acquisition plan unavailable, importing what is on disk",
			slog.Any("err", err))
		return false
	}

	store := runplan.TrackingStore{}
	if _, _, running, activeErr := store.Active(r.Context()); activeErr == nil && running {
		respondJSON(w, http.StatusConflict, map[string]string{"error": runplan.ErrPlanRunning.Error()})
		return true
	}

	sourceURL := config.CricsheetSourceURL()
	dataDir := dataset.Dir()
	acquisition := dataacquire.PlanAcquisition(
		sourceURL,
		dataset.StagingDir(),
		dataDir,
		dataset.Inspect(dataDir).MatchFiles,
		body.Refresh,
	)

	skip := map[string]string{}
	if acquisition.SkipFetch != "" {
		skip["fetch"] = acquisition.SkipFetch
	}
	if acquisition.SkipExtract != "" {
		skip["extract"] = acquisition.SkipExtract
	}

	slog.Info("import: starting acquisition plan",
		slog.String("source_url", sourceURL),
		slog.Bool("refresh", body.Refresh),
		slog.Any("skipped", skip))
	a.StartImportPlan(steps, skip, StepRequest{PlaceholdersFielding: body.PlaceholdersFielding})

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"plan":   runplan.PlanImport,
		"steps":  []string{"fetch", "extract", "import"},
		// The source and the skips are in the response because the operator clicked
		// one button and three things are about to happen: saying which of them will
		// actually run, and why the others will not, is the difference between a plan
		// and a surprise.
		"source_url": sourceURL,
		"skipped":    skip,
	})
	return true
}
