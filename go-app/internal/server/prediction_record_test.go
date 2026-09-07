package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/predictions"
	"github.com/umayangag/cric-flow/go-app/internal/predictions/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// servedResult is one finished prediction, as predictteam.PredictTeams would have
// returned it: stamped with the run and date it was served from (P1-5), both sides
// resolved, and a headline probability with the model behind it.
func servedResult() *predictteam.Result {
	return &predictteam.Result{
		ServedRatings: predictteam.ServedRatings{
			RunID:          "20260907T062657Z-6b16045e",
			RatingsThrough: "2026-09-02",
		},
		Team1Side: predictteam.ResolvedSide{
			ClubID: 43, Name: "India", Gender: "male", DisplayName: "India (male)",
		},
		Team2Side: predictteam.ResolvedSide{
			ClubID: 51, Name: "Australia", Gender: "male", DisplayName: "Australia (male)",
		},
		Team1:          []predictteam.SelectedPlayer{{PlayerID: 7, PlayerName: "A Player", Runs: 31}},
		Team2:          []predictteam.SelectedPlayer{{PlayerID: 9, PlayerName: "B Player", Runs: 28}},
		Selection:      predictteam.SelectionSummary{Objective: "win", Optimised: true},
		Forecast:       predictteam.ForecastSummary{Source: "simulator"},
		WinProbability: predictteam.WinProbabilitySummary{Team1: 0.62, Source: "display", PredictedWinner: "India"},
	}
}

// servedRequest is the request that answer answered.
func servedRequest() predictTeamRequest {
	return predictTeamRequest{
		Format:    " t20i ",
		Team1ID:   43,
		Team2ID:   51,
		MatchDate: "2026-09-10",
	}
}

// servedPredictionBody is the answer as a caller decodes it: the `record` block, and
// enough of the prediction to show that a store failure left it alone.
type servedPredictionBody struct {
	Record         predictions.RecordBlock           `json:"record"`
	WinProbability predictteam.WinProbabilitySummary `json:"win_probability"`
	Team1          []predictteam.SelectedPlayer      `json:"team1"`
}

// servedMatchDate is the fixture's match day, parsed the way the handler parses it.
func servedMatchDate(t *testing.T) time.Time {
	t.Helper()
	parsed, err := parseMatchDate("2026-09-10")
	require.NoError(t, err)
	return parsed
}

// The record holds the answer that was served, and the row it holds is the one the answer
// names: the payload in the store and the body on the wire are the same bytes, so "reading
// a prediction back reproduces what was served" is a property of the code rather than a
// claim about two encoders agreeing (P2-3, gate clause 3).
func TestRespondWithRecordedPrediction_StoresTheBytesItServes(t *testing.T) {
	t.Parallel()
	recorder := mocks.NewMockRecorder(t)
	var filed predictions.Prediction
	recorder.EXPECT().Record(mock.Anything, mock.Anything).
		Run(func(_ context.Context, prediction predictions.Prediction) { filed = prediction }).
		Return(nil)
	app := &App{predictionRecorderStore: recorder}
	rec := httptest.NewRecorder()

	app.respondWithRecordedPrediction(rec, jsonPredictRequest(`{}`),
		servedRequest(), servedMatchDate(t), servedResult())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, string(filed.Payload)+"\n", rec.Body.String(),
		"the store keeps the bytes the caller received, whole")

	var served servedPredictionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &served))
	assert.True(t, served.Record.Stored)
	assert.Equal(t, filed.ID, served.Record.ID, "the answer names the row that holds it")
	assert.Equal(t, filed.IssuedAt.Format(time.RFC3339), served.Record.IssuedAt)
	assert.Empty(t, served.Record.Reason)
}

// Every column beside the two documents is read off the answer or the request that
// produced it, and nothing is derived that they do not already say.
func TestRespondWithRecordedPrediction_ColumnsComeOffTheAnswer(t *testing.T) {
	t.Parallel()
	recorder := mocks.NewMockRecorder(t)
	var filed predictions.Prediction
	recorder.EXPECT().Record(mock.Anything, mock.Anything).
		Run(func(_ context.Context, prediction predictions.Prediction) { filed = prediction }).
		Return(nil)
	app := &App{predictionRecorderStore: recorder}

	app.respondWithRecordedPrediction(httptest.NewRecorder(), jsonPredictRequest(`{}`),
		servedRequest(), servedMatchDate(t), servedResult())

	assert.Equal(t, "20260907T062657Z-6b16045e", filed.RunID)
	assert.Equal(t, "2026-09-02", filed.RatingsThrough.Format(time.DateOnly))
	assert.Equal(t, "T20I", filed.FormatCode, "the format is folded the way the prediction folded it")
	assert.Equal(t, int64(43), filed.Team1OppositionID)
	assert.Equal(t, int64(51), filed.Team2OppositionID)
	assert.Equal(t, "male", filed.Gender)
	assert.Equal(t, "2026-09-10", filed.MatchDate.Format(time.DateOnly))
	assert.Equal(t, "win", filed.SelectionObjective)
	assert.InDelta(t, 0.62, filed.WinProbabilityTeam1, 1e-9)
	assert.Equal(t, "display", filed.WinProbabilitySource)
	assert.JSONEq(t, `{"format":" t20i ","team1_id":43,"team2_id":51,"team1":"","team2":"",
		"team1_gender":"","team2_gender":"","venue":"","match_date":"2026-09-10",
		"extra_team1":null,"extra_team2":null,"team1_xi":null,"team2_xi":null,
		"min_bowlers":0,"require_keeper":null,"team1_bats_first":null,
		"team1_pool":{},"team2_pool":{}}`, string(filed.Request))
}

// An eleven the caller pinned is filed like any other answer, as a scenario.
//
// P2-4 counts what this store holds, so a Play-mode re-score that went unrecorded would be
// a prediction the user saw and the record could not account for. `selection.objective` is
// what separates it from a forecast about a fixture: `fixed` is listed and never scored.
func TestRespondWithRecordedPrediction_RecordsAPlayModeRescoreAsAScenario(t *testing.T) {
	t.Parallel()
	recorder := mocks.NewMockRecorder(t)
	var filed predictions.Prediction
	recorder.EXPECT().Record(mock.Anything, mock.Anything).
		Run(func(_ context.Context, prediction predictions.Prediction) { filed = prediction }).
		Return(nil)
	app := &App{predictionRecorderStore: recorder}
	result := servedResult()
	result.Selection = predictteam.SelectionSummary{Objective: predictteam.SelectionObjectiveFixed}
	body := servedRequest()
	body.Team1XI = []int64{1, 2, 3}
	body.Team2XI = []int64{4, 5, 6}

	app.respondWithRecordedPrediction(httptest.NewRecorder(), jsonPredictRequest(`{}`),
		body, servedMatchDate(t), result)

	assert.Equal(t, predictteam.SelectionObjectiveFixed, filed.SelectionObjective)
	assert.Contains(t, string(filed.Request), `"team1_xi":[1,2,3]`,
		"the eleven the caller built is in the request the record kept")
}

// A store that fails does not cost the caller the answer, and does not do it quietly: the
// prediction is served with `record.stored: false` and the store's own reason on it (§8.7).
func TestRespondWithRecordedPrediction_ServesTheAnswerAndNamesTheStoreFailure(t *testing.T) {
	t.Parallel()
	recorder := mocks.NewMockRecorder(t)
	recorder.EXPECT().Record(mock.Anything, mock.Anything).
		Return(assert.AnError)
	app := &App{predictionRecorderStore: recorder}
	rec := httptest.NewRecorder()

	app.respondWithRecordedPrediction(rec, jsonPredictRequest(`{}`),
		servedRequest(), servedMatchDate(t), servedResult())

	require.Equal(t, http.StatusOK, rec.Code, "a bookkeeping failure never refuses a prediction")
	var served servedPredictionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &served))
	assert.False(t, served.Record.Stored)
	assert.Contains(t, served.Record.Reason, assert.AnError.Error())
	assert.Empty(t, served.Record.ID, "there is no row to name")
	assert.InDelta(t, 0.62, served.WinProbability.Team1, 1e-9, "the prediction is untouched")
	assert.Len(t, served.Team1, 1)
}

// A refusal is not a prediction, so nothing is filed for one.
//
// The mock is given no expectation at all: any call to Record would fail this test, which
// is the assertion — a record that held refusals would count answers nobody was given.
func TestPredictTeamSelectionHandler_RecordsNothingWhenItRefuses(t *testing.T) {
	t.Parallel()
	app := &App{predictionRecorderStore: mocks.NewMockRecorder(t)}
	rec := httptest.NewRecorder()

	app.predictTeamSelectionHandler(rec, jsonPredictRequest(
		`{"format":"T20I","team2_id":7,"match_date":"2026-09-10"}`))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "INVALID_PARAM", decodeAPIError(t, rec).Code)
}

// The default record is the database-backed one, so a process that wires nothing still
// files what it serves.
func TestApp_PredictionRecordDefaultsToTheDatabaseStore(t *testing.T) {
	t.Parallel()
	app := &App{}

	assert.IsType(t, &db.PredictionStore{}, app.predictionRecorder())
	assert.IsType(t, &db.PredictionStore{}, app.predictionReader())
}
