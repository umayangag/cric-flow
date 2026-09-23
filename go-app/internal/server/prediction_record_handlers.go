package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/umayangag/cric-flow/go-app/internal/predictions"
)

// Paging defaults for GET /api/predictions. A record read by one operator is read a
// screenful at a time; the cap is there so a mistyped limit cannot ask for every row and
// its payload-free listing at once.
const (
	predictionPageDefault = 20
	predictionPageMax     = 200
)

// predictionSummary is one row of the record's listing: what a resolver joins on and what
// a reader sorts and filters by, and no payload.
//
// The payload is left out on purpose. A listing is the record's index; twenty answers with
// twenty-two players' forecasts each is megabytes of JSON to render a table of dates. The
// claim itself is one request away, by id.
type predictionSummary struct {
	ID             string `json:"id"`
	IssuedAt       string `json:"issued_at"`
	RunID          string `json:"run_id"`
	RatingsThrough string `json:"ratings_through"`
	Format         string `json:"format"`
	// The fixture: the two sides as opposition ids, the gender and the day. Together they
	// are the key an imported match is matched on when the record scores it (P2-4).
	Team1OppositionID int64  `json:"team1_opposition_id"`
	Team2OppositionID int64  `json:"team2_opposition_id"`
	Gender            string `json:"gender"`
	MatchDate         string `json:"match_date"`
	// Objective is "win", "ratings" or "fixed" — the last being an eleven the caller
	// pinned in Play mode, which is a scenario and not a forecast about a fixture.
	Objective            string  `json:"objective"`
	WinProbabilityTeam1  float64 `json:"win_probability_team1"`
	WinProbabilitySource string  `json:"win_probability_source"`
}

// predictionListResponse is one page of the record, newest first.
type predictionListResponse struct {
	Predictions []predictionSummary `json:"predictions"`
	// Total is how many rows the page was taken out of, so a surface can say "20 of 143"
	// rather than leaving a reader to guess whether the list is the whole record.
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// storedPredictionResponse is one stored answer, whole.
//
// `payload` is the bytes the caller received, and `request` the request they sent. They
// are raw JSON rather than re-modelled types: the record's job is to show what was
// claimed, and a struct here would silently drop any field a later payload gained.
type storedPredictionResponse struct {
	predictionSummary
	Request json.RawMessage `json:"request"`
	Payload json.RawMessage `json:"payload"`
}

// newPredictionSummary renders one stored row for the wire.
func newPredictionSummary(stored predictions.Prediction) predictionSummary {
	return predictionSummary{
		ID:                   stored.ID,
		IssuedAt:             stored.IssuedAt.UTC().Format(time.RFC3339),
		RunID:                stored.RunID,
		RatingsThrough:       stored.RatingsThrough.Format(time.DateOnly),
		Format:               stored.FormatCode,
		Team1OppositionID:    stored.Team1OppositionID,
		Team2OppositionID:    stored.Team2OppositionID,
		Gender:               stored.Gender,
		MatchDate:            stored.MatchDate.Format(time.DateOnly),
		Objective:            stored.SelectionObjective,
		WinProbabilityTeam1:  stored.WinProbabilityTeam1,
		WinProbabilitySource: stored.WinProbabilitySource,
	}
}

// listPredictionsHandler answers GET /api/predictions: the record, newest first, paged.
func (a *App) listPredictionsHandler(w http.ResponseWriter, r *http.Request) {
	query, apiErr := parsePredictionPage(r)
	if apiErr != nil {
		writeJSON(w, http.StatusBadRequest, *apiErr)
		return
	}
	page, err := a.predictionReader().List(r.Context(), query)
	if err != nil {
		slog.Error("listPredictions: reading the record failed", slog.Any("err", err))
		respondErr(w, err)
		return
	}
	summaries := make([]predictionSummary, 0, len(page.Predictions))
	for i := range page.Predictions {
		summaries = append(summaries, newPredictionSummary(page.Predictions[i]))
	}
	writeJSON(w, http.StatusOK, predictionListResponse{
		Predictions: summaries,
		Total:       page.Total,
		Limit:       query.Limit,
		Offset:      query.Offset,
	})
}

// getPredictionHandler answers GET /api/predictions/{id}: one stored answer, as served.
func (a *App) getPredictionHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	// A prediction id is a UUID (predictions.NewID); the column it is compared against is
	// typed uuid, and Postgres refuses a value that is not one with 22P02 rather than
	// answering zero rows. Left unchecked, that reached the caller as a 500 INTERNAL for
	// what is really the same "no prediction under this id" as a well-formed id nothing
	// matches (GO-16), so it is refused here, before a query is issued, the same way
	// getPlayerHandler and getMatchHandler already refuse an unparseable id.
	if _, err := uuid.Parse(id); err != nil {
		slog.Info("getPrediction: invalid prediction id", slog.String("prediction_id", id), slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}
	stored, err := a.predictionReader().Get(r.Context(), id)
	if errors.Is(err, predictions.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, apiError{
			Code:    "PREDICTION_NOT_FOUND",
			Message: "no prediction was issued under id " + id,
			Hint:    "ids come from the `record` block of a prediction, or from GET /api/predictions",
		})
		return
	}
	if err != nil {
		slog.Error("getPrediction: reading the record failed",
			slog.String("prediction_id", id), slog.Any("err", err))
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, storedPredictionResponse{
		predictionSummary: newPredictionSummary(*stored),
		Request:           stored.Request,
		Payload:           stored.Payload,
	})
}

// parsePredictionPage reads the page a listing asks for.
//
// A limit or offset that is not a number is refused rather than defaulted: a caller who
// typed `limit=twenty` and silently got twenty rows out of a record of hundreds would have
// no way to tell that the page they are reading is not the page they asked for.
func parsePredictionPage(r *http.Request) (predictions.Query, *apiError) {
	query := predictions.Query{Limit: predictionPageDefault}
	limit, apiErr := parsePositiveParam(r.URL.Query().Get("limit"), "limit")
	if apiErr != nil {
		return predictions.Query{}, apiErr
	}
	if limit > 0 {
		query.Limit = min(limit, predictionPageMax)
	}
	offset, apiErr := parsePositiveParam(r.URL.Query().Get("offset"), "offset")
	if apiErr != nil {
		return predictions.Query{}, apiErr
	}
	query.Offset = offset
	return query, nil
}

// parsePositiveParam reads a whole number that may not be negative. An absent value is
// zero, which each caller reads as its own default.
func parsePositiveParam(raw, name string) (int, *apiError) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, &apiError{
			Code:    "INVALID_PARAM",
			Message: name + " must be a whole number that is not negative",
		}
	}
	return value, nil
}
