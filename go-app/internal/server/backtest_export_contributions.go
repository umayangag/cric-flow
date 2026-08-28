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
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
	"golang.org/x/sync/errgroup"
)

const contributionsCSVFilenamePrefix = "backtest_contributions"

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

// exportContribsMatchPrep holds pre-computed data for one match before ML prediction.
type exportContribsMatchPrep struct {
	matchID  int64
	cutoff   time.Time
	squad    []int64
	features map[int64]map[string]float64
}

// runExportContributionsWork runs the export in the calling goroutine (used by the background job).
// Returns path, row count, and error. Keeper lookup failure is returned as error so the job can report it to the user.
//
// The function uses three phases to minimise cross-service round-trips:
//  1. Parallel DB prep  — cutoff, squad, features for every match.
//  2. Single batch ML call — all predictions in one HTTP request; falls back to
//     parallel per-match calls if the batch endpoint is unavailable.
//  3. Parallel result assembly — actuals + player-results for each match.
func runExportContributionsWork(
	ctx context.Context,
	body exportContributionsRequest,
) (path string, rows int, err error) {
	format := strings.TrimSpace(strings.ToUpper(body.Format))
	cfg := config.Load()
	concurrency := config.BacktestExportContributionsConcurrency(cfg)

	// Phase 1: Prep all matches in parallel (DB only, no ML).
	preps := make([]exportContribsMatchPrep, len(body.MatchIDs))
	g1, gCtx1 := errgroup.WithContext(ctx)
	g1.SetLimit(concurrency)
	for i, mid := range body.MatchIDs {
		i, mid := i, mid
		g1.Go(func() error {
			cutoff, err := getBacktestMatchDateFunc(gCtx1, mid)
			if err != nil {
				slog.Warn("export-contributions prep failed (match_date)", "match_id", mid, "err", err)
				return nil
			}
			squad, err := getBacktestSquadPlayerIDsFunc(gCtx1, mid, cutoff, format)
			if err != nil {
				slog.Warn("export-contributions prep failed (squad)", "match_id", mid, "err", err)
				return nil
			}
			feats, err := getBacktestFeaturesAtCutoffFunc(gCtx1, cutoff, squad, mid, format)
			if err != nil {
				slog.Warn("export-contributions prep failed (features)", "match_id", mid, "err", err)
				return nil
			}
			preps[i] = exportContribsMatchPrep{matchID: mid, cutoff: cutoff, squad: squad, features: feats}
			return nil
		})
	}
	if err := g1.Wait(); err != nil {
		return "", 0, err
	}

	// Collect successfully prepped matches.
	var batchInputs []BatchPredictPlayersInput
	var validIndices []int
	for i, p := range preps {
		if p.cutoff.IsZero() {
			continue
		}
		batchInputs = append(batchInputs, BatchPredictPlayersInput{
			Cutoff:    p.cutoff,
			Format:    format,
			PlayerIDs: p.squad,
			Features:  p.features,
		})
		validIndices = append(validIndices, i)
	}

	// Phase 2: ML predictions — batch when available, per-match fallback otherwise.
	allPlayers := exportContribsBatchPredict(ctx, preps, batchInputs, validIndices, format, body)

	if len(allPlayers) == 0 {
		return writeEmptyContributionsCSV()
	}

	playerIDs := uniquePlayerIDs(allPlayers)
	keeperMap, err := db.ListPlayerIsWicketKeeper(ctx, playerIDs)
	if err != nil {
		slog.Warn("export-contributions list keeper failed", "err", err)
		return "", 0, err
	}

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

// exportContribsBatchPredict attempts a single batch ML call for all prepped
// matches.  Falls back to parallel per-match doEvaluateWork calls when the
// batch endpoint is unavailable.
func exportContribsBatchPredict(
	ctx context.Context,
	preps []exportContribsMatchPrep,
	batchInputs []BatchPredictPlayersInput,
	validIndices []int,
	format string,
	body exportContributionsRequest,
) []BacktestPlayerResult {
	if len(batchInputs) == 0 {
		return nil
	}
	cfg := config.Load()
	concurrency := config.BacktestExportContributionsConcurrency(cfg)
	batchResults, batchErr := mlBacktestPredictBatchFunc(ctx, batchInputs)
	if batchErr != nil {
		slog.Warn("batch predict unavailable for export, falling back to per-match calls", slog.Any("err", batchErr))
		return exportContribsFallback(ctx, body, format)
	}

	// Phase 3: Assemble player results from batch predictions + actuals.
	var allPlayers []BacktestPlayerResult
	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)
	for bi, vi := range validIndices {
		bi, vi := bi, vi
		g.Go(func() error {
			p := preps[vi]
			preds := batchResults[bi]
			actuals, err := getBacktestPlayerActualsForMatchFunc(gCtx, p.matchID)
			if err != nil {
				slog.Warn("export-contributions actuals failed", "match_id", p.matchID, "err", err)
				return nil
			}
			players, _ := computePlayerResultsAndMetrics(p.squad, preds, actuals)
			mu.Lock()
			allPlayers = append(allPlayers, players...)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return allPlayers
}

// exportContribsFallback uses the original per-match doEvaluateWork calls when
// the batch endpoint is unavailable.
func exportContribsFallback(
	ctx context.Context,
	body exportContributionsRequest,
	format string,
) []BacktestPlayerResult {
	cfg := config.Load()
	concurrency := config.BacktestExportContributionsConcurrency(cfg)
	team1 := strings.TrimSpace(body.Team1)
	team2 := strings.TrimSpace(body.Team2)
	var allPlayers []BacktestPlayerResult
	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)
	for _, mid := range body.MatchIDs {
		mid := mid
		g.Go(func() error {
			matchIDStr := strconv.FormatInt(mid, 10)
			resp, err := doEvaluateWork(gCtx, format, team1, team2, matchIDStr, nil)
			if err != nil {
				slog.Warn("export-contributions evaluate failed", "match_id", mid, "err", err)
				return nil
			}
			mu.Lock()
			allPlayers = append(allPlayers, resp.Players...)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return allPlayers
}

func writeEmptyContributionsCSV() (string, int, error) {
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

func uniquePlayerIDs(players []BacktestPlayerResult) []int64 {
	seen := make(map[int64]bool, len(players))
	ids := make([]int64, 0, len(players))
	for _, p := range players {
		if !seen[p.PlayerID] {
			seen[p.PlayerID] = true
			ids = append(ids, p.PlayerID)
		}
	}
	return ids
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
