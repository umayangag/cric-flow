package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// A prediction's date travels on the wire, and so does its refusal (P1-5).

// The ml-service refusal for stale ratings, as the wire carries it (H-11).
var staleRatingsRefusal = &mlServiceError{
	Endpoint: "/xi/optimize",
	Status:   http.StatusServiceUnavailable,
	Code:     "RATINGS_STALE",
	Message:  "ratings run through 2026-09-02 (40 days old, limit 14)",
	Hint:     "run the retrain step, then reload -- or raise XI_RATINGS_MAX_AGE_DAYS if this is deliberate",
}

func TestRelayStatus_PassesRefusalsThroughAndBlamesTheDependencyForTheRest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		upstream int
		want     int
	}{
		{name: "a caller's mistake stays a 4xx", upstream: http.StatusNotFound, want: http.StatusNotFound},
		{
			name:     "a deliberate refusal stays a 503, because this service cannot predict either",
			upstream: http.StatusServiceUnavailable,
			want:     http.StatusServiceUnavailable,
		},
		{name: "ml-service crashing is this service's dependency failing", upstream: 500, want: http.StatusBadGateway},
		{name: "an upstream gateway failure is likewise", upstream: http.StatusBadGateway, want: http.StatusBadGateway},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, relayStatus(tc.upstream))
		})
	}
}

// H-11 through go-app: the refusal reaches the caller as the 503 ml-service wrote, with the
// code, the date, the age against the limit and the step that fixes it — never a number.
func TestRespondPredictErr_StaleRatingsAre503WithTheCodeOnTheWire(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()

	respondPredictErr(rec, fmt.Errorf("select India (men): %w", staleRatingsRefusal))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "RATINGS_STALE", body.Code)
	assert.Contains(t, body.Message, "2026-09-02")
	assert.Contains(t, body.Message, "40 days old, limit 14")
	assert.Contains(t, body.Hint, "retrain")
}

// A prediction whose calls straddled a reload has no single date to carry, so it is not
// carried at all; the caller is told to run it again.
func TestRespondPredictErr_ARunChangeMidPredictionIs409NamingBothRuns(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	err := fmt.Errorf("simulate match: %w", &predictteam.ServedRunChangedError{
		Was: predictteam.ServedRatings{RunID: "run-a", RatingsThrough: "2026-09-02"},
		Now: predictteam.ServedRatings{RunID: "run-b", RatingsThrough: "2026-09-09"},
	})

	respondPredictErr(rec, err)

	assert.Equal(t, http.StatusConflict, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "SERVED_RUN_CHANGED", body.Code)
	assert.Contains(t, body.Message, "run-a")
	assert.Contains(t, body.Message, "run-b")
	assert.Contains(t, body.Hint, "run it again")
}

// The date and the run are top-level, required fields of the prediction payload: a client
// that copies the number out has the date beside it, and there is no omitempty for a blank
// to hide behind.
func TestPredictionPayload_CarriesTheDateAndRunAtTheTopLevel(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	result := &predictteam.Result{ServedRatings: servedFromTheDevRun}

	writeJSON(rec, http.StatusOK, result)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.JSONEq(t, `"2026-09-02"`, string(payload["ratings_through"]))
	assert.JSONEq(t, `"20260906T083819Z-36689f80"`, string(payload["run_id"]))

	rec = httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, &predictteam.Result{})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.Contains(t, payload, "ratings_through", "a blank date is on the wire as blank, never absent")
	assert.Contains(t, payload, "run_id")
}

func TestPredictPerformance_MapsTheForecastAndItsStamp(t *testing.T) {
	t.Parallel()
	client, captured := xiCaptureServer(t, `{
	  "players": [{"player_id": "a1", "side": 1, "p_bats": 1.0, "p_bowls": 0.2,
	               "runs": {"q10": 3, "median": 26, "q90": 71}, "balls_faced": {"q10": 8, "median": 44, "q90": 110},
	               "runs_conceded": {"q10": 10, "median": 33, "q90": 60},
	               "wickets": {"expected": 1.4, "p0": 0.3, "p1": 0.4, "p2_plus": 0.3}, "catches_expected": 0.5}],
	  "innings_marginalised": true,
	  "unknown_player_ids": [],
	  "served_ratings": {"run_id": "20260906T083819Z-36689f80", "ratings_through": "2026-09-02"}
	}`)

	result, err := client.PredictPerformance(context.Background(), predictteam.XIPerformanceRequest{
		Format:          "TEST",
		Team1PlayerKeys: []string{"a1"},
		Team2PlayerKeys: []string{"b1"},
		Team1ID:         7,
	})

	require.NoError(t, err)
	assert.Equal(t, float64(7), (*captured)["team1_id"])
	assert.True(t, result.InningsMarginalised)
	require.Len(t, result.Players, 1)
	assert.InDelta(t, 26, result.Players[0].Runs.Median, 1e-9)
	assert.InDelta(t, 1.4, result.Players[0].Wickets, 1e-9)
	assert.Equal(t, servedFromTheDevRun, result.Served)
}
