package server

import (
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

const contributionsCSVFilename = "backtest_contributions.csv"

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
	Format   string  `json:"format"`
	Team1    string  `json:"team1"`
	Team2    string  `json:"team2"`
	MatchIDs []int64 `json:"match_ids"`
}

// exportContributionsResponse is the JSON response.
type exportContributionsResponse struct {
	Path string `json:"path"`
	Rows int    `json:"rows"`
}

// backtestExportContributionsHandler handles POST /api/backtest/export-contributions.
// Runs evaluate for each match_id, builds contribution rows (bat_score, bowl_score, field_score, is_keeper, format, target),
// writes CSV to output dir, returns path and row count.
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
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"})
		return
	}
	if len(body.MatchIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "match_ids must be non-empty"})
		return
	}

	ctx := r.Context()
	var allPlayers []BacktestPlayerResult
	for _, mid := range body.MatchIDs {
		matchIDStr := strconv.FormatInt(mid, 10)
		resp, err := doEvaluateWork(ctx, format, team1, team2, matchIDStr, false, nil)
		if err != nil {
			slog.Warn("export-contributions evaluate failed", "match_id", mid, "err", err)
			continue
		}
		allPlayers = append(allPlayers, resp.Players...)
	}

	if len(allPlayers) == 0 {
		writeJSON(w, http.StatusOK, exportContributionsResponse{Path: "", Rows: 0})
		return
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
		keeperMap = map[int64]bool{}
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

	rows := buildContributionRows(allPlayers, keeperMap, format, batDiv, wicketDiv, econBase, fieldDiv)
	outDir := config.DefaultExportDir()
	if err := os.MkdirAll(outDir, 0755); err != nil {
		respondErr(w, err)
		return
	}
	csvPath := filepath.Join(outDir, contributionsCSVFilename)
	if err := writeContributionsCSV(csvPath, rows); err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, exportContributionsResponse{Path: csvPath, Rows: len(rows)})
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

		batScore := math.Min(1, runs/batDiv)
		wickPart := math.Min(1, wickets/wicketDiv)
		econPart := math.Max(0, 1-(econ/econBase))
		bowlScore := (wickPart + econPart) / 2
		fieldScore := math.Min(1, (catches+runOuts*1.5)/fieldDiv)

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
	defer w.Flush()
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
	return nil
}
