package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	formatsPkg "github.com/umayangag/cric-flow/go-app/internal/formats"

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

// respondTrainingDataErr maps known training-data errors to appropriate HTTP status and message.
// Format-not-found (e.g. migrations not run or match_format empty) -> 400; DB not ready -> 503; else 500.
func respondTrainingDataErr(w http.ResponseWriter, err error, format string) {
	if err == nil {
		return
	}
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
		hint := "Ensure migrations are applied and match_format is populated (" + strings.Join(
			formatsPkg.CanonicalCodes(),
			", ",
		) + ")."
		// Echo the requested format only when it is a known code, to avoid reflecting input.
		if formatsPkg.IsCanonical(format) || format == "all" {
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
