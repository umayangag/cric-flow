package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/predictions"
	"github.com/umayangag/cric-flow/go-app/internal/predictions/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/trackrecord"
)

// scriptedMatchLookup answers every fixture with the same matches: enough for the handler
// test, which is about the wiring, not the rules (those are internal/trackrecord's).
type scriptedMatchLookup struct {
	matches []trackrecord.PlayedMatch
	err     error
}

func (s scriptedMatchLookup) FindMatches(context.Context, trackrecord.Fixture) ([]trackrecord.PlayedMatch, error) {
	return s.matches, s.err
}

func storedForecast(id string, probability float64) predictions.Prediction {
	return predictions.Prediction{
		ID:                   id,
		IssuedAt:             time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC),
		RunID:                "20260907T062657Z-6b16045e",
		RatingsThrough:       time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		FormatCode:           "T20I",
		Team1OppositionID:    4,
		Team2OppositionID:    54,
		Gender:               "male",
		MatchDate:            time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		SelectionObjective:   "win",
		WinProbabilityTeam1:  probability,
		WinProbabilitySource: "display",
		Payload: json.RawMessage(`{"team1_side": {"display_name": "Australia"}, "team2_side": {"display_name": "England"},
			"team1": [{"player_id": 1}], "team2": [{"player_id": 2}]}`),
	}
}

func TestTrackRecordHandler_ScoresTheRecordAgainstTheMatchesFound(t *testing.T) {
	reader := mocks.NewMockReader(t)
	reader.EXPECT().All(mock.Anything).Return([]predictions.Prediction{storedForecast("p-1", 0.3)}, nil)
	winner := int64(54)
	app := &App{predictionReaderStore: reader, matchLookupStore: scriptedMatchLookup{matches: []trackrecord.PlayedMatch{{
		MatchID: 77, WinnerOppositionID: &winner,
		Innings:        []trackrecord.PlayedInnings{{Number: 1, BattingOppositionID: 4, Runs: 150}, {Number: 2, BattingOppositionID: 54, Runs: 151}},
		FieldedPlayers: map[int64]int64{1: 4, 2: 54},
	}}}}
	rec := httptest.NewRecorder()

	app.trackRecordHandler(rec, httptest.NewRequest(http.MethodGet, "/api/track-record", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var record trackrecord.Record
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &record))
	assert.Equal(t, 1, record.Total)
	assert.Equal(t, 1, record.States[trackrecord.StateScored])
	require.Len(t, record.Predictions, 1)
	assert.Equal(t, trackrecord.StateScored, record.Predictions[0].State)
	assert.Equal(t, "Australia", record.Predictions[0].Team1.Name)
	require.NotNil(t, record.Win.Overall.Brier)
	assert.InDelta(t, 0.09, *record.Win.Overall.Brier, 1e-9, "0.3 for a side that lost")
	assert.Equal(t, 1, record.Win.Overall.N)
}

func TestTrackRecordHandler_AnUnreadableRecordIsAnError(t *testing.T) {
	reader := mocks.NewMockReader(t)
	reader.EXPECT().All(mock.Anything).Return(nil, errors.New("db pool not initialized"))
	app := &App{predictionReaderStore: reader}
	rec := httptest.NewRecorder()

	app.trackRecordHandler(rec, httptest.NewRequest(http.MethodGet, "/api/track-record", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "db pool not initialized")
}

func TestTrackRecordHandler_AMatchLookupFailureIsAnErrorNotAnUnresolvedRecord(t *testing.T) {
	reader := mocks.NewMockReader(t)
	reader.EXPECT().All(mock.Anything).Return([]predictions.Prediction{storedForecast("p-1", 0.3)}, nil)
	app := &App{predictionReaderStore: reader, matchLookupStore: scriptedMatchLookup{err: errors.New("connection refused")}}
	rec := httptest.NewRecorder()

	app.trackRecordHandler(rec, httptest.NewRequest(http.MethodGet, "/api/track-record", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "connection refused")
}

func TestMatchLookup_DefaultsToTheDatabase(t *testing.T) {
	var app *App
	assert.NotNil(t, app.matchLookup())
	assert.NotNil(t, (&App{}).matchLookup())
}
