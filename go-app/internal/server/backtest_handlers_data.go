package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	exq "github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)

// trainingDataResponse is the JSON shape for GET /api/backtest/training-data (for ML service train-on-the-fly).
type trainingDataResponse struct {
	Batting  trainingDataPart `json:"batting"`
	Bowling  trainingDataPart `json:"bowling"`
	Fielding trainingDataPart `json:"fielding"`
	Extras   trainingDataPart `json:"extras"`
	Win      trainingDataPart `json:"win"`
	Innings  trainingDataPart `json:"innings"`
}

type trainingDataPart struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// allowedTrainingDataFormats is the fixed set of format codes safe to include in API error hints (avoids reflected input).
var allowedTrainingDataFormats = map[string]bool{
	"TEST": true, "ODI": true, "T20": true, "T20I": true, "all": true,
}

// respondTrainingDataErr maps known training-data errors to appropriate HTTP status and message.
// Format-not-found (e.g. migrations not run or match_format empty) -> 400; DB not ready -> 503; else 500.
func respondTrainingDataErr(w http.ResponseWriter, err error, format string) {
	if err == nil {
		return
	}
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
		hint := "Ensure migrations are applied and match_format is populated (TEST, ODI, T20, T20I)."
		if allowedTrainingDataFormats[format] {
			hint += " Format requested: " + format
		}
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "FORMAT_NOT_FOUND",
			Message: "format not found or database not ready for training-data",
			Hint:    hint,
		})
		slog.Info("training-data: format not found or no rows", slog.String("format", format), slog.Any("err", err))
		return
	}
	if strings.Contains(err.Error(), "db pool not initialized") {
		writeJSON(w, http.StatusServiceUnavailable, apiError{
			Code:    "SERVICE_UNAVAILABLE",
			Message: "database not connected",
			Hint:    "Go-app may still be starting; retry shortly.",
		})
		slog.Warn("training-data: db pool not initialized", slog.Any("err", err))
		return
	}
	respondErr(w, err)
}

// allowedTrainingDataSections is the set of valid section names for training-data ?sections= (reduces go-app/DB load when only one model is needed).
var allowedTrainingDataSections = map[string]bool{
	"batting": true, "bowling": true, "fielding": true, "extras": true, "win": true, "innings": true,
}

// backtestScorecardHandler handles GET /api/backtest/scorecard?match_id=...
// It returns the match scorecard (innings, batting and bowling card) for the given match.
func (a *App) backtestScorecardHandler(w http.ResponseWriter, r *http.Request) {
	matchIDStr := strings.TrimSpace(r.URL.Query().Get("match_id"))
	if matchIDStr == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "match_id is required"})
		return
	}
	matchID, err := strconv.ParseInt(matchIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid match_id"})
		return
	}
	card, err := db.GetMatchScorecard(r.Context(), matchID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, apiError{Code: "NOT_FOUND", Message: "match not found"})
			return
		}
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

// backtestTrainingDataHandler handles GET /api/backtest/training-data?cutoff=...&format=...&sections=...
// cutoff (RFC3339) is required. format: use "all" (or omit) for all matches before cutoff; use a specific code (T20, ODI, etc.) to filter by that format.
// sections: optional comma-separated list (batting,bowling,fielding,extras,win). If omitted, all sections are returned (legacy). If set, only those sections are queried to reduce go-app and DB CPU during auto-tune.
func (a *App) backtestTrainingDataHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error(
				"training-data: panic recovered",
				slog.Any("panic", rec),
				slog.String("stack", string(debug.Stack())),
			)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}()

	cutoffStr := strings.TrimSpace(r.URL.Query().Get("cutoff"))
	if cutoffStr == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "cutoff is required (RFC3339)"})
		return
	}
	cutoff, err := time.Parse(time.RFC3339, cutoffStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "cutoff must be RFC3339"})
		return
	}
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	useAll := format == "" || strings.EqualFold(format, "all")
	formatForErr := format
	if formatForErr == "" {
		formatForErr = "all"
	}
	sectionsParam := strings.TrimSpace(r.URL.Query().Get("sections"))
	wantSection := map[string]bool{}
	if sectionsParam != "" {
		for _, s := range strings.Split(sectionsParam, ",") {
			s = strings.TrimSpace(strings.ToLower(s))
			if allowedTrainingDataSections[s] {
				wantSection[s] = true
			}
		}
	}
	runAllSections := len(wantSection) == 0

	slog.Info(
		"training-data: request start",
		slog.String("cutoff", cutoffStr),
		slog.String("format", format),
		slog.String("sections", sectionsParam),
		slog.Bool("use_all", useAll),
		slog.Bool("run_all_sections", runAllSections),
	)

	var batRows, bowlRows, fieldRows, extrasRows, winRows, inningsRows [][]string
	type sectionLoader struct {
		name       string
		rows       *[][]string
		loadAll    func(context.Context, time.Time) ([][]string, error)
		loadFormat func(context.Context, string, time.Time) ([][]string, error)
	}
	loaders := []sectionLoader{
		{"batting", &batRows, exq.BattingTrainingRows, exq.BattingTrainingRowsWithFormat},
		{"bowling", &bowlRows, exq.BowlingTrainingRows, exq.BowlingTrainingRowsWithFormat},
		{"fielding", &fieldRows, exq.FieldingTrainingRows, exq.FieldingTrainingRowsWithFormat},
		{"extras", &extrasRows, exq.ExtrasTrainingRows, exq.ExtrasTrainingRowsWithFormat},
		{"win", &winRows, exq.WinTrainingRows, exq.WinTrainingRowsWithFormat},
		{"innings", &inningsRows, exq.InningsTrainingRows, exq.InningsTrainingRowsWithFormat},
	}
	for _, loader := range loaders {
		if runAllSections || wantSection[loader.name] {
			t0 := time.Now()
			var loadErr error
			if useAll {
				*loader.rows, loadErr = loader.loadAll(r.Context(), cutoff)
			} else {
				*loader.rows, loadErr = loader.loadFormat(r.Context(), format, cutoff)
			}
			elapsed := time.Since(t0)
			rowCount := len(*loader.rows)
			if loadErr != nil {
				slog.Error(
					"training-data: section load failed",
					slog.String("section", loader.name),
					slog.Duration("elapsed", elapsed),
					slog.Any("err", loadErr),
				)
				respondTrainingDataErr(w, loadErr, formatForErr)
				return
			}
			slog.Info(
				"training-data: section loaded",
				slog.String("section", loader.name),
				slog.Int("rows", rowCount),
				slog.Duration("elapsed_ms", elapsed),
			)
		}
	}
	part := func(rows [][]string) (headers []string, data [][]string) {
		if len(rows) > 0 {
			return rows[0], rows[1:]
		}
		return nil, nil
	}
	batH, batD := part(batRows)
	bowlH, bowlD := part(bowlRows)
	fieldH, fieldD := part(fieldRows)
	extrasH, extrasD := part(extrasRows)
	winH, winD := part(winRows)
	inningsH, inningsD := part(inningsRows)

	slog.Info(
		"training-data: all sections ready, writing response",
		slog.Int("batting_rows", len(batD)),
		slog.Int("bowling_rows", len(bowlD)),
		slog.Int("fielding_rows", len(fieldD)),
		slog.Int("extras_rows", len(extrasD)),
		slog.Int("win_rows", len(winD)),
		slog.Int("innings_rows", len(inningsD)),
	)
	writeJSON(w, http.StatusOK, trainingDataResponse{
		Batting:  trainingDataPart{Headers: batH, Rows: batD},
		Bowling:  trainingDataPart{Headers: bowlH, Rows: bowlD},
		Fielding: trainingDataPart{Headers: fieldH, Rows: fieldD},
		Extras:   trainingDataPart{Headers: extrasH, Rows: extrasD},
		Win:      trainingDataPart{Headers: winH, Rows: winD},
		Innings:  trainingDataPart{Headers: inningsH, Rows: inningsD},
	})
	slog.Info("training-data: response written successfully")
}

// matchesAfterResponse is the JSON shape for GET /api/backtest/matches (walk-forward).
type matchesAfterResponse struct {
	Matches []matchAfterItem `json:"matches"`
}

type matchAfterItem struct {
	MatchID   int64  `json:"match_id"`
	MatchDate string `json:"match_date"` // RFC3339
}

// backtestMatchesHandler handles GET /api/backtest/matches?after=...&format=...&limit=...
// Used by walk-forward: list match_id and match_date for matches strictly after the cutoff.
func (a *App) backtestMatchesHandler(w http.ResponseWriter, r *http.Request) {
	afterStr := strings.TrimSpace(r.URL.Query().Get("after"))
	if afterStr == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "after is required (RFC3339)"})
		return
	}
	after, err := time.Parse(time.RFC3339, afterStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "after must be RFC3339"})
		return
	}
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if format == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format is required (e.g. T20, ODI)"},
		)
		return
	}
	cfg := config.Load()
	limit := config.BacktestListDefaultLimit(cfg)
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
			maxLimit := config.BacktestListMaxLimit(cfg)
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(r.Context(), format)
	if err != nil {
		respondErr(w, err)
		return
	}
	items, err := db.ListMatchIDsAfter(r.Context(), formatIDs, after, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	out := make([]matchAfterItem, 0, len(items))
	for _, it := range items {
		out = append(out, matchAfterItem{MatchID: it.MatchID, MatchDate: it.MatchDate.Format(time.RFC3339)})
	}
	writeJSON(w, http.StatusOK, matchesAfterResponse{Matches: out})
}

// backtestHoldoutDataHandler handles GET /api/backtest/holdout-data?cutoff=...&format=...&limit=...
// Returns training-data-shaped JSON for matches strictly after cutoff (features computed at cutoff) for walk-forward.
func (a *App) backtestHoldoutDataHandler(w http.ResponseWriter, r *http.Request) {
	cutoffStr := strings.TrimSpace(r.URL.Query().Get("cutoff"))
	if cutoffStr == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "cutoff is required (RFC3339)"})
		return
	}
	cutoff, err := time.Parse(time.RFC3339, cutoffStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "cutoff must be RFC3339"})
		return
	}
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if format == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format is required (e.g. T20, ODI)"},
		)
		return
	}
	cfg := config.Load()
	limit := config.BacktestListDefaultLimit(cfg)
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
			maxLimit := config.BacktestListMaxLimit(cfg)
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}
	batRows, err := exq.BattingHoldoutRows(r.Context(), format, cutoff, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	bowlRows, err := exq.BowlingHoldoutRows(r.Context(), format, cutoff, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	fieldRows, err := exq.FieldingHoldoutRows(r.Context(), format, cutoff, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	part := func(rows [][]string) (headers []string, data [][]string) {
		if len(rows) > 0 {
			return rows[0], rows[1:]
		}
		return nil, nil
	}
	batH, batD := part(batRows)
	bowlH, bowlD := part(bowlRows)
	fieldH, fieldD := part(fieldRows)
	writeJSON(w, http.StatusOK, trainingDataResponse{
		Batting:  trainingDataPart{Headers: batH, Rows: batD},
		Bowling:  trainingDataPart{Headers: bowlH, Rows: bowlD},
		Fielding: trainingDataPart{Headers: fieldH, Rows: fieldD},
		Extras:   trainingDataPart{},
		Win:      trainingDataPart{},
	})
}
