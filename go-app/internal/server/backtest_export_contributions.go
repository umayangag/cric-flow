package server

import (
	"context"
	"encoding/csv"
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
	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
	"golang.org/x/sync/errgroup"
)

const (
	contributionsCSVFilenamePrefix = "backtest_contributions"
	exportContributionsConcurrency = 8 // limit concurrent doEvaluateWork calls per export job
)

// contributionRow is one row for the combination meta-model CSV.
type contributionRow struct {
	BatScore   float64
	BowlScore  float64
	FieldScore float64
	IsKeeper   int
	Format     string
	Target     float64
}

// exportContributionsRequest is the body for POST /api/backtest/export-contributions.
type exportContributionsRequest struct {
	Format          string  `json:"format"`
	Team1           string  `json:"team1"`
	Team2           string  `json:"team2"`
	MatchIDs        []int64 `json:"match_ids"`
	UseUnifiedModel bool    `json:"use_unified_model,omitempty"`
}

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
			resp, err := doEvaluateWork(gCtx, format, team1, team2, matchIDStr, body.UseUnifiedModel, nil)
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
	out := make([]contributionRow, 0, len(players))
	for _, p := range players {
		pred := p.Predicted
		act := p.Actual
		if pred == nil || act == nil {
			continue
		}
		runs := pred["runs"]
		wickets := pred["wickets"]
		econ := pred["economy"]
		catches := pred["catches"]
		runOuts := pred["run_outs"]
		actualRuns := act["runs"]

		batScore := teamselect.NormalizeBatScore(runs, batDiv)
		bowlScore := teamselect.NormalizeBowlScore(wickets, econ, wicketDiv, econBase)
		fieldScore := teamselect.NormalizeFieldScore(catches, runOuts, fieldDiv)

		isKeeper := 0
		if keeperMap[p.PlayerID] {
			isKeeper = 1
		}
		target := 0.0
		if batDiv > 0 {
			target = actualRuns / batDiv
		}
		out = append(out, contributionRow{
			BatScore:   batScore,
			BowlScore:  bowlScore,
			FieldScore: fieldScore,
			IsKeeper:   isKeeper,
			Format:     format,
			Target:     target,
		})
	}
	return out
}

func writeContributionsCSV(path string, rows []contributionRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	header := []string{"bat_score", "bowl_score", "field_score", "is_keeper", "format", "target"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		record := []string{
			strconv.FormatFloat(r.BatScore, 'f', -1, 64),
			strconv.FormatFloat(r.BowlScore, 'f', -1, 64),
			strconv.FormatFloat(r.FieldScore, 'f', -1, 64),
			strconv.Itoa(r.IsKeeper),
			r.Format,
			strconv.FormatFloat(r.Target, 'f', -1, 64),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
