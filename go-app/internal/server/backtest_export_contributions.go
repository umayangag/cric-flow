package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
	"golang.org/x/sync/errgroup"
)

const (
	contributionsCSVFilenamePrefix = "backtest_contributions"
	exportContributionsConcurrency = 8 // limit concurrent doEvaluateWork calls per export job
)

// contributionRow is one row for the combination meta-model CSV.
// Types contributionRow and exportContributionsRequest are defined as aliases in backtest_types.go
// pointing to services/backtest package.

// backtestExportContributionsHandler handles POST /api/backtest/export-contributions.
// Starts a background job that runs evaluate for each match_id, builds contribution rows, and writes CSV.
// Returns 202 Accepted with job_id; client polls GET /api/backtest/export-contributions-status?job_id=...
func (a *App) backtestExportContributionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST required"})
		return
	}
	var body exportContributionsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_JSON", Message: err.Error()})
		return
	}
	format := strings.TrimSpace(strings.ToUpper(body.Format))
	team1 := strings.TrimSpace(body.Team1)
	team2 := strings.TrimSpace(body.Team2)
	if format == "" || team1 == "" || team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}
	if len(body.MatchIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "match_ids must be non-empty"})
		return
	}
	maxMatchIDs := config.EffectiveExportMaxMatchIDs(config.Load())
	if len(body.MatchIDs) > maxMatchIDs {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{
				Code:    "INVALID_PARAM",
				Message: "match_ids exceeds limit of " + strconv.Itoa(maxMatchIDs) + "; process in batches",
			},
		)
		return
	}

	jobID, err := startExportContributionsJob(body)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// backtestExportContributionsStatusHandler handles GET /api/backtest/export-contributions-status?job_id=...
func (a *App) backtestExportContributionsStatusHandler(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "job_id is required"})
		return
	}
	snap, ok := getExportContributionsJobStatus(jobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, apiError{Code: "NOT_FOUND", Message: "job not found (expired or invalid id)"})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// runExportContributionsWork runs the export in the calling goroutine (used by the background job).
// Returns path, row count, and error. Keeper lookup failure is returned as error so the job can report it to the user.
func runExportContributionsWork(
	ctx context.Context,
	body exportContributionsRequest,
) (path string, rows int, err error) {
	format := strings.TrimSpace(strings.ToUpper(body.Format))
	team1 := strings.TrimSpace(body.Team1)
	team2 := strings.TrimSpace(body.Team2)

	// Evaluate matches in parallel with a concurrency limit to avoid overloading external services.
	var allPlayers []BacktestPlayerResult
	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(exportContributionsConcurrency)
	for _, mid := range body.MatchIDs {
		mid := mid
		g.Go(func() error {
			matchIDStr := strconv.FormatInt(mid, 10)
			resp, err := doEvaluateWork(gCtx, format, team1, team2, matchIDStr, body.UseUnifiedModel, false, nil)
			if err != nil {
				slog.Warn("export-contributions evaluate failed", "match_id", mid, "err", err)
				return nil // continue: don't fail the whole job for one match
			}
			mu.Lock()
			allPlayers = append(allPlayers, resp.Players...)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return "", 0, err
	}

	if len(allPlayers) == 0 {
		outDir := config.DefaultExportDir()
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return "", 0, err
		}
		csvPath := filepath.Join(outDir, contributionsCSVFilenamePrefix+".csv")
		if err := writeContributionsCSV(csvPath, nil); err != nil {
			return "", 0, err
		}
		return csvPath, 0, nil
	}

	playerIDs := make([]int64, 0, len(allPlayers))
	seen := make(map[int64]bool)
	for _, p := range allPlayers {
		if !seen[p.PlayerID] {
			seen[p.PlayerID] = true
			playerIDs = append(playerIDs, p.PlayerID)
		}
	}
	keeperMap, err := db.ListPlayerIsWicketKeeper(ctx, playerIDs)
	if err != nil {
		slog.Warn("export-contributions list keeper failed", "err", err)
		return "", 0, err
	}

	cfg := config.Load()
	batDiv, wicketDiv, econBase, fieldDiv := config.EffectiveScoreNormParams(cfg, format)
	if batDiv <= 0 {
		batDiv = config.DefaultScoreNormBatDivisor
	}
	if wicketDiv <= 0 {
		wicketDiv = config.DefaultScoreNormWicketDivisor
	}
	if econBase <= 0 {
		econBase = config.DefaultScoreNormEconBase
	}
	if fieldDiv <= 0 {
		fieldDiv = config.DefaultScoreNormFieldDivisor
	}

	rowList := buildContributionRows(allPlayers, keeperMap, format, batDiv, wicketDiv, econBase, fieldDiv)
	outDir := config.DefaultExportDir()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", 0, err
	}
	csvPath := filepath.Join(outDir, contributionsCSVFilenamePrefix+".csv")
	if err := writeContributionsCSV(csvPath, rowList); err != nil {
		return "", 0, err
	}
	return csvPath, len(rowList), nil
}

func buildContributionRows(
	players []BacktestPlayerResult,
	keeperMap map[int64]bool,
	format string,
	batDiv, wicketDiv, econBase, fieldDiv float64,
) []contributionRow {
	return backtest.BuildContributionRows(players, keeperMap, format, batDiv, wicketDiv, econBase, fieldDiv)
}

func writeContributionsCSV(path string, rows []contributionRow) error {
	return backtest.WriteContributionsCSV(path, rows)
}
