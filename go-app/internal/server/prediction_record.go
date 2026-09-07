package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/predictions"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// predictionResponse is what a caller receives: the prediction, and what became of the
// attempt to file it (P2-3).
//
// The Result is embedded, so `record` is one more field beside `selection`, `forecast`
// and the rest rather than a wrapper that would have moved every existing field down a
// level and broken every reader of this endpoint.
type predictionResponse struct {
	*predictteam.Result
	Record predictions.RecordBlock `json:"record"`
}

// predictionRecorder is what the prediction path files an answer through: one method, and
// no way from here to read the record back.
//
// A nil field is the process default — the database-backed record — so a handler built
// without one still records, which is how every other store in this package is reached
// (candidate_handlers.go builds the retirement ledger the same way). Tests set the field
// to a mock.
func (a *App) predictionRecorder() predictions.Recorder {
	if a != nil && a.predictionRecorderStore != nil {
		return a.predictionRecorderStore
	}
	return db.NewPredictionStore()
}

// predictionReader is the record's read side, on the same terms.
func (a *App) predictionReader() predictions.Reader {
	if a != nil && a.predictionReaderStore != nil {
		return a.predictionReaderStore
	}
	return db.NewPredictionStore()
}

// respondWithRecordedPrediction files the answer and then serves it.
//
// The order matters and is the point of the item: the row is written before the caller
// sees the number, so the record cannot be missing an answer somebody acted on. Every
// successful answer is filed, Play-mode re-scores included — the record is what makes
// P2-4's counting honest, and an eleven the caller pinned is on the record as a scenario
// (`selection.objective` = "fixed") rather than left off it.
//
// A store failure does not refuse the prediction. The refusals this endpoint already
// makes — a stale rating state, a reload mid-assembly, a cross-gender fixture, an
// incomplete eleven — are all cases where the *answer would be wrong or meaningless*. A
// failure to file changes nothing about the answer's truth; it damages the completeness of
// the record. Refusing here would turn a bookkeeping outage into an outage of the only
// thing this product does, and would put a new single point of failure in front of a path
// that has none. So the answer is served and the failure rides on it (§8.7): `record` says
// `stored: false` with the store's own reason, where the person reading the number can see
// that this one will not be on the record — never only in a log line.
func (a *App) respondWithRecordedPrediction(
	w http.ResponseWriter,
	r *http.Request,
	body predictTeamRequest,
	matchDate time.Time,
	result *predictteam.Result,
) {
	issuedAt := time.Now().UTC()
	response := predictionResponse{
		Result: result,
		Record: predictions.RecordBlock{
			Stored:   true,
			ID:       predictions.NewID(),
			IssuedAt: issuedAt.Format(time.RFC3339),
		},
	}

	// The bytes are built before the insert because the row holds them: what is stored is
	// the answer the caller receives, `record` block and all, so a stored prediction can
	// be shown exactly as it was served rather than reassembled from parts.
	served, err := json.MarshalIndent(response, "", "  ")
	if err == nil {
		err = a.fileIssuedPrediction(r.Context(), response.Record, body, matchDate, result, served)
	}
	if err != nil {
		slog.Error("prediction record: served an answer this store did not keep",
			slog.String("prediction_id", response.Record.ID), slog.Any("err", err))
		response.Record = predictions.RecordBlock{Stored: false, Reason: err.Error()}
		writeJSON(w, http.StatusOK, response)
		return
	}
	writeJSONBytes(w, http.StatusOK, served)
}

// fileIssuedPrediction turns one served answer into the row the record holds.
//
// Every column beside the two documents is read off the answer itself or off the request
// that produced it; nothing is derived that they do not already say. The rating state
// comes from the payload's own stamp (P1-5) and never from a status call, which would
// describe whatever is loaded at the moment of the call rather than what served this
// prediction.
func (a *App) fileIssuedPrediction(
	ctx context.Context,
	block predictions.RecordBlock,
	body predictTeamRequest,
	matchDate time.Time,
	result *predictteam.Result,
	served []byte,
) error {
	issuedAt, err := time.Parse(time.RFC3339, block.IssuedAt)
	if err != nil {
		return err
	}
	ratingsThrough, err := time.Parse(time.DateOnly, result.RatingsThrough)
	if err != nil {
		return err
	}
	// The request is stored as this API parsed it, so the POST body and the GET query
	// form are recorded as the same request — which is what the service answered.
	request, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return a.predictionRecorder().Record(ctx, predictions.Prediction{
		ID:                   block.ID,
		IssuedAt:             issuedAt,
		RunID:                result.RunID,
		RatingsThrough:       ratingsThrough,
		FormatCode:           predictteam.NormalizeFormat(body.Format),
		Team1OppositionID:    result.Team1Side.ClubID,
		Team2OppositionID:    result.Team2Side.ClubID,
		Gender:               result.Team1Side.Gender,
		MatchDate:            matchDate,
		SelectionObjective:   result.Selection.Objective,
		WinProbabilityTeam1:  result.WinProbability.Team1,
		WinProbabilitySource: result.WinProbability.Source,
		Request:              request,
		Payload:              served,
	})
}
