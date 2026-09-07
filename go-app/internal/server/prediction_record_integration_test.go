package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// The prediction record end to end against a real database and a scripted ml-service
// (P2-3): a served answer is on the record before the caller sees it, it reads back as the
// bytes that were served, and a refused prediction leaves the record empty.
//
// These truncate. Run them against a scratch database, never one holding an import:
// `make -C go-app test-db`; dbtest.SkipUnlessScratchDatabase refuses the working one.

// canonicalJSON re-encodes a JSON document into one deterministic form: keys sorted,
// whitespace normalised.
//
// It is how the round trip is asserted, and it is worth saying why. `payload` is a `jsonb`
// column, and `jsonb` stores a JSON *value* — every field, every number, every null — and
// not the bytes it arrived as: it drops insignificant whitespace and orders keys its own
// way. So the strongest true statement about the round trip is that the value that comes
// back is the value that was served, and this is what asserts it. The other half of the
// claim is asserted where it *is* about bytes: the unit test in prediction_record_test.go
// checks that the bytes handed to the store are exactly the bytes written to the caller.
func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var value any
	require.NoError(t, json.Unmarshal(raw, &value))
	canonical, err := json.Marshal(value)
	require.NoError(t, err)
	return string(canonical)
}

// storedPredictionCount is what the record holds, read straight from the table.
func storedPredictionCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, db.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM issued_prediction`).Scan(&count))
	return count
}

// The gate's clause 3, end to end: the answer is stored, and reading it back reproduces
// exactly what was served, with the run and the date it was served from.
func TestPredictionRecord_AServedAnswerReadsBackAsItWasServed_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, nil)}
	predicted := httptest.NewRecorder()

	app.predictTeamSelectionHandler(predicted, predictRequestFor(fixture))

	require.Equal(t, http.StatusOK, predicted.Code, predicted.Body.String())
	var served servedPredictionBody
	require.NoError(t, json.Unmarshal(predicted.Body.Bytes(), &served))
	require.True(t, served.Record.Stored, "a served answer is on the record")
	require.NotEmpty(t, served.Record.ID)

	read := httptest.NewRecorder()
	app.getPredictionHandler(read, getPredictionRequest(served.Record.ID))

	require.Equal(t, http.StatusOK, read.Code, read.Body.String())
	var stored struct {
		ID             string          `json:"id"`
		RunID          string          `json:"run_id"`
		RatingsThrough string          `json:"ratings_through"`
		MatchDate      string          `json:"match_date"`
		Format         string          `json:"format"`
		Gender         string          `json:"gender"`
		Objective      string          `json:"objective"`
		Team1ID        int64           `json:"team1_opposition_id"`
		Team2ID        int64           `json:"team2_opposition_id"`
		Payload        json.RawMessage `json:"payload"`
		Request        json.RawMessage `json:"request"`
	}
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &stored))
	assert.Equal(t, canonicalJSON(t, predicted.Body.Bytes()), canonicalJSON(t, stored.Payload),
		"the record reproduces the answer that was served")
	assert.Equal(t, served.Record.ID, stored.ID)
	assert.Equal(t, "20260906T083819Z-36689f80", stored.RunID)
	assert.Equal(t, "2026-09-02", stored.RatingsThrough)
	assert.Equal(t, fixture.matchDate, stored.MatchDate)
	assert.Equal(t, "TEST", stored.Format)
	assert.Equal(t, "male", stored.Gender)
	assert.Equal(t, "ratings", stored.Objective, "the fixture's format is rating-ordered (H-17)")
	assert.Equal(t, fixture.team1ID, stored.Team1ID)
	assert.Equal(t, fixture.team2ID, stored.Team2ID)
	assert.Contains(t, canonicalJSON(t, stored.Request), `"format":"TEST"`)
}

// The listing shows what was issued, newest first, with the columns a resolver joins on.
func TestPredictionRecord_ListsWhatWasIssued_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, nil)}

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		app.predictTeamSelectionHandler(rec, predictRequestFor(fixture))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	listed := httptest.NewRecorder()
	app.listPredictionsHandler(listed, httptest.NewRequest(http.MethodGet, "/api/predictions", nil))

	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var page struct {
		Predictions []struct {
			ID                   string  `json:"id"`
			IssuedAt             string  `json:"issued_at"`
			Format               string  `json:"format"`
			Objective            string  `json:"objective"`
			WinProbabilityTeam1  float64 `json:"win_probability_team1"`
			WinProbabilitySource string  `json:"win_probability_source"`
		} `json:"predictions"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &page))
	assert.Equal(t, 2, page.Total)
	require.Len(t, page.Predictions, 2)
	assert.GreaterOrEqual(t, page.Predictions[0].IssuedAt, page.Predictions[1].IssuedAt, "newest first")
	assert.Equal(t, "TEST", page.Predictions[0].Format)
	assert.Equal(t, "ratings", page.Predictions[0].Objective)
	assert.InDelta(t, 0.6, page.Predictions[0].WinProbabilityTeam1, 1e-9)
	assert.Equal(t, "display", page.Predictions[0].WinProbabilitySource)
}

// A refusal is not a prediction: nothing is filed for one, so the record cannot count an
// answer nobody was given.
func TestPredictionRecord_ARefusedPredictionStoresNothing_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, staleRatingsRefusal)}
	rec := httptest.NewRecorder()

	app.predictTeamSelectionHandler(rec, predictRequestFor(fixture))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, 0, storedPredictionCount(t), "a refusal leaves the record empty")
}

// A Play-mode re-score is on the record like any other answer, as a scenario: P2-4 counts
// what this store holds, and an eleven the user built is listed and never scored.
func TestPredictionRecord_APlayModeRescoreIsRecordedAsAScenario_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	xi1, xi2 := seededPlayerIDs(t, "1"), seededPlayerIDs(t, "2")
	require.Len(t, xi1, 11)
	app := &App{mlClient: scriptedMLService(t, nil)}
	rec := httptest.NewRecorder()

	app.predictTeamSelectionHandler(rec, playPredictRequest(fixture, xi1, xi2, nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var served servedPredictionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &served))
	require.True(t, served.Record.Stored)

	read := httptest.NewRecorder()
	app.getPredictionHandler(read, getPredictionRequest(served.Record.ID))

	require.Equal(t, http.StatusOK, read.Code, read.Body.String())
	var stored struct {
		Objective string          `json:"objective"`
		Request   json.RawMessage `json:"request"`
	}
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &stored))
	assert.Equal(t, "fixed", stored.Objective)
	assert.Contains(t, canonicalJSON(t, stored.Request), `"team1_xi"`,
		"the eleven the user built is in the request the record kept")
}
