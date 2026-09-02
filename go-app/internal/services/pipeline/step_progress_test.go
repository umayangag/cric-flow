package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withMLService points MLServiceBaseURL at a test server for the duration of a test.
func withMLService(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Setenv("ML_SERVICE_URL", srv.URL)
	t.Cleanup(srv.Close)
}

func TestFetchStepProgress_ReturnsThePublishedEvent(t *testing.T) {
	var gotQuery string
	withMLService(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"v":1,"step":"train_batting","phase":"cv","current":3,"total":5}`))
	})

	progress, err := FetchStepProgress(context.Background(), "train_batting")
	require.NoError(t, err)
	require.NotNil(t, progress)
	assert.Equal(t, "cv", progress["phase"])
	assert.Contains(t, gotQuery, "step=train_batting")
}

// TestFetchStepProgress_EmptyIsNotAnError: a reachable service saying "nothing
// published yet" is a normal answer on a step that has just started.
func TestFetchStepProgress_EmptyIsNotAnError(t *testing.T) {
	withMLService(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})

	progress, err := FetchStepProgress(context.Background(), "train_batting")
	require.NoError(t, err)
	assert.Nil(t, progress)
}

// TestFetchStepProgress_UnreachableIsDistinguishable is the whole point of returning
// an error rather than nil: "could not ask" and "nothing to report" render the same
// empty panel, and only one of them is worth telling the operator about.
func TestFetchStepProgress_UnreachableIsDistinguishable(t *testing.T) {
	t.Setenv("ML_SERVICE_URL", "http://127.0.0.1:1")

	progress, err := FetchStepProgress(context.Background(), "train_batting")
	require.ErrorIs(t, err, ErrProgressUnavailable)
	assert.Nil(t, progress)
}

func TestFetchStepProgress_NonOKStatusIsUnavailable(t *testing.T) {
	for name, code := range map[string]int{
		"unauthorized": http.StatusUnauthorized,
		"not found":    http.StatusNotFound,
		"server error": http.StatusInternalServerError,
	} {
		t.Run(name, func(t *testing.T) {
			withMLService(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			})

			_, err := FetchStepProgress(context.Background(), "train_batting")
			require.ErrorIs(t, err, ErrProgressUnavailable)
		})
	}
}

func TestFetchStepProgress_UnreadableBodyIsUnavailable(t *testing.T) {
	withMLService(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	})

	_, err := FetchStepProgress(context.Background(), "train_batting")
	require.ErrorIs(t, err, ErrProgressUnavailable)
}

// TestFetchStepProgress_EmptyStepAsksNothing: a command outside the registry has no
// step id, and a request per SSE tick for an answer that cannot exist is waste.
func TestFetchStepProgress_EmptyStepAsksNothing(t *testing.T) {
	asked := 0
	withMLService(t, func(w http.ResponseWriter, _ *http.Request) {
		asked++
		_, _ = w.Write([]byte(`{}`))
	})

	progress, err := FetchStepProgress(context.Background(), "  ")
	require.NoError(t, err)
	assert.Nil(t, progress)
	assert.Zero(t, asked)
}

// TestFetchStepProgress_EscapesTheStepID: step ids come from the registry today, but
// the query is built by string concatenation and a value with a & in it would forge a
// second parameter.
func TestFetchStepProgress_EscapesTheStepID(t *testing.T) {
	var gotStep string
	withMLService(t, func(w http.ResponseWriter, r *http.Request) {
		gotStep = r.URL.Query().Get("step")
		_, _ = w.Write([]byte(`{}`))
	})

	_, err := FetchStepProgress(context.Background(), "retrain&run_id=other")
	require.NoError(t, err)
	assert.Equal(t, "retrain&run_id=other", gotStep, "the whole value stays one parameter")
}

// TestCallMLTrainEndpointWithResult_ReturnsTheSummary: the summary comes back on the
// response because there is no afterwards to poll in — the progress file is removed
// as the run ends, so this is the only moment the outcome still exists.
func TestCallMLTrainEndpointWithResult_ReturnsTheSummary(t *testing.T) {
	withMLService(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		_, _ = w.Write([]byte(`{"status":"ok","step":"retrain","summary":{"formats_completed":4,"saved":4}}`))
	})

	result, err := CallMLTrainEndpointWithResult(context.Background(), "retrain", "?cutoff=2026-01-01")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Status)
	assert.Equal(t, float64(4), result.Summary["formats_completed"])
}

func TestCallMLTrainEndpointWithResult_NoSummaryIsFine(t *testing.T) {
	withMLService(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","step":"evaluate"}`))
	})

	result, err := CallMLTrainEndpointWithResult(context.Background(), "evaluate", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Summary)
}

// TestCallMLTrainEndpointWithResult_UnreadableBodyDoesNotFailTheRun: the step
// succeeded. Losing the summary is not losing the run, and turning it into an error
// would report work that happened as work that did not.
func TestCallMLTrainEndpointWithResult_UnreadableBodyDoesNotFailTheRun(t *testing.T) {
	withMLService(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	})

	result, err := CallMLTrainEndpointWithResult(context.Background(), "retrain", "")
	require.NoError(t, err, "a 200 is a success whatever the body says")
	assert.Nil(t, result)
}

func TestCallMLTrainEndpointWithResult_PropagatesAFailure(t *testing.T) {
	withMLService(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":{"code":"CONTRIBUTIONS_CSV_MISSING","message":"nope"}}`))
	})

	result, err := CallMLTrainEndpointWithResult(context.Background(), "evaluate", "")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "CONTRIBUTIONS_CSV_MISSING")
}

// TestCallMLTrainEndpoint_KeepsItsErrorOnlyContract for callers that never wanted the
// body.
func TestCallMLTrainEndpoint_KeepsItsErrorOnlyContract(t *testing.T) {
	withMLService(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","step":"retrain"}`))
	})

	require.NoError(t, CallMLTrainEndpoint(context.Background(), "retrain", ""))
}
