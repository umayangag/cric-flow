package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/predictions"
	"github.com/umayangag/cric-flow/go-app/internal/predictions/mocks"
)

// storedPrediction is one row as the record holds it.
func storedPrediction(id string) predictions.Prediction {
	return predictions.Prediction{
		ID:                   id,
		IssuedAt:             time.Date(2026, 9, 7, 6, 26, 57, 0, time.UTC),
		RunID:                "20260907T062657Z-6b16045e",
		RatingsThrough:       time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		FormatCode:           "T20I",
		Team1OppositionID:    43,
		Team2OppositionID:    51,
		Gender:               "male",
		MatchDate:            time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		SelectionObjective:   "win",
		WinProbabilityTeam1:  0.62,
		WinProbabilitySource: "display",
		Request:              json.RawMessage(`{"format":"T20I"}`),
		Payload:              json.RawMessage(`{"win_probability":{"team1":0.62}}`),
	}
}

// getPredictionRequest is a read of one stored answer, with the id in the path the way
// the router puts it there.
func getPredictionRequest(id string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/predictions/"+id, nil)
	return mux.SetURLVars(request, map[string]string{"id": id})
}

// A stored answer comes back whole: the two documents as raw JSON, and the columns a
// resolver joins on beside them.
func TestGetPredictionHandler_ReturnsTheStoredAnswer(t *testing.T) {
	t.Parallel()
	reader := mocks.NewMockReader(t)
	stored := storedPrediction("2b0f6c0e-1a4d-4e0a-9d7a-91f2b5c7a001")
	reader.EXPECT().Get(mock.Anything, stored.ID).Return(&stored, nil)
	app := &App{predictionReaderStore: reader}
	rec := httptest.NewRecorder()

	app.getPredictionHandler(rec, getPredictionRequest(stored.ID))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, stored.ID, body["id"])
	assert.Equal(t, "2026-09-07T06:26:57Z", body["issued_at"])
	assert.Equal(t, "2026-09-02", body["ratings_through"])
	assert.Equal(t, "2026-09-10", body["match_date"])
	assert.Equal(t, "T20I", body["format"])
	assert.Equal(t, "win", body["objective"])
	assert.InDelta(t, 0.62, body["win_probability_team1"], 1e-9)
	assert.Equal(t, map[string]any{"format": "T20I"}, body["request"])
	assert.Equal(t, map[string]any{"win_probability": map[string]any{"team1": 0.62}}, body["payload"])
}

// An id the record does not hold is a 404 naming it, not an empty 200: "we never issued
// that" and "we issued it and it said nothing" are different answers. The id is a
// well-formed UUID nothing was ever stored under -- a malformed id is a different case,
// pinned by TestGetPredictionHandler_ANonUUIDIdIsRefusedWithoutQueryingTheStore below.
func TestGetPredictionHandler_SaysWhenTheRecordHoldsNoSuchAnswer(t *testing.T) {
	t.Parallel()
	const missing = "8f14e45f-ceea-467e-bc4a-085d5a56cd01"
	reader := mocks.NewMockReader(t)
	reader.EXPECT().Get(mock.Anything, missing).Return(nil, predictions.ErrNotFound)
	app := &App{predictionReaderStore: reader}
	rec := httptest.NewRecorder()

	app.getPredictionHandler(rec, getPredictionRequest(missing))

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "PREDICTION_NOT_FOUND", body.Code)
	assert.Contains(t, body.Message, missing)
}

// GO-16: a prediction id is a UUID, and the column it is compared against is typed uuid --
// a value that is not one made Postgres answer 22P02, which repo_prediction.go's Get did
// not recognise as "not found" (it is not pgx.ErrNoRows), so it reached respondErr as a
// bare error and answered 500 INTERNAL for what is, to a caller, indistinguishable from a
// well-formed id nothing matches. The reader is a strict mock with no expectations set: if
// the handler still queried the store for a non-UUID id, the mock's own
// AssertExpectations(t) cleanup would fail this test.
func TestGetPredictionHandler_ANonUUIDIdIsRefusedWithoutQueryingTheStore(t *testing.T) {
	t.Parallel()
	reader := mocks.NewMockReader(t)
	app := &App{predictionReaderStore: reader}
	rec := httptest.NewRecorder()

	app.getPredictionHandler(rec, getPredictionRequest("not-a-uuid"))

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	body := decodeAPIError(t, rec)
	assert.Equal(t, "BAD_REQUEST", body.Code)
}

// The listing is the record's index: the join columns, the page it is, and the count it
// was taken out of — and no payloads.
func TestListPredictionsHandler_ListsThePageWithTheCountItCameFrom(t *testing.T) {
	t.Parallel()
	reader := mocks.NewMockReader(t)
	reader.EXPECT().List(mock.Anything, predictions.Query{Limit: 2, Offset: 4}).
		Return(predictions.Page{
			Predictions: []predictions.Prediction{storedPrediction("a"), storedPrediction("b")},
			Total:       143,
		}, nil)
	app := &App{predictionReaderStore: reader}
	rec := httptest.NewRecorder()

	app.listPredictionsHandler(rec, httptest.NewRequest(
		http.MethodGet, "/api/predictions?limit=2&offset=4", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Predictions []map[string]any `json:"predictions"`
		Total       int              `json:"total"`
		Limit       int              `json:"limit"`
		Offset      int              `json:"offset"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, 143, body.Total)
	assert.Equal(t, 2, body.Limit)
	assert.Equal(t, 4, body.Offset)
	require.Len(t, body.Predictions, 2)
	assert.Equal(t, "a", body.Predictions[0]["id"])
	assert.NotContains(t, body.Predictions[0], "payload")
}

func TestParsePredictionPage_ReadsThePageOrRefusesToGuess(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		query     string
		want      predictions.Query
		wantError bool
	}{
		{
			name:  "no page asked for is the default page",
			query: "",
			want:  predictions.Query{Limit: predictionPageDefault},
		},
		{
			name:  "a page as asked for",
			query: "?limit=5&offset=10",
			want:  predictions.Query{Limit: 5, Offset: 10},
		},
		{
			name:  "a limit past the cap is the cap",
			query: "?limit=100000",
			want:  predictions.Query{Limit: predictionPageMax},
		},
		{
			name:      "a limit nobody can read is refused, not defaulted",
			query:     "?limit=twenty",
			wantError: true,
		},
		{
			name:      "a negative offset is refused",
			query:     "?offset=-1",
			wantError: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/api/predictions"+tc.query, nil)

			query, apiErr := parsePredictionPage(request)

			assertPageParse(t, tc.wantError, tc.want, query, apiErr)
		})
	}
}

// assertPageParse keeps the branch out of the test case, which is the rule the suite
// follows: a case asserts, it does not decide.
func assertPageParse(
	t *testing.T,
	wantError bool,
	want, got predictions.Query,
	apiErr *apiError,
) {
	t.Helper()
	if wantError {
		require.NotNil(t, apiErr)
		assert.Equal(t, "INVALID_PARAM", apiErr.Code)
		return
	}
	require.Nil(t, apiErr)
	assert.Equal(t, want, got)
}
