package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

func TestDataFeedsHandler_StatesTheAllowlist(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	(&App{}).dataFeedsHandler(rec, httptest.NewRequest(http.MethodGet, "/ops/data/feeds", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Feeds        []dataacquire.Feed `json:"feeds"`
		AllowedHosts []string           `json:"allowed_hosts"`
		StagingDir   string             `json:"staging_dir"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body.Feeds)
	assert.Contains(t, body.AllowedHosts, "cricsheet.org",
		"the UI states the rule up front rather than letting the operator discover it by refusal")
	assert.NotEmpty(t, body.StagingDir)
}

// TestDataFetchHandler_RejectsOffAllowlistBeforeStartingAJob checks the refusal
// happens at the edge: no tracked job, no staging file, and a message that says what
// is allowed.
func TestDataFetchHandler_RejectsOffAllowlistBeforeStartingAJob(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"metadata service": `{"url":"https://169.254.169.254/latest/meta-data/"}`,
		"loopback":         `{"url":"https://127.0.0.1/admin"}`,
		"lookalike host":   `{"url":"https://cricsheet.org.attacker.example/all_json.zip"}`,
		"plaintext":        `{"url":"http://cricsheet.org/downloads/all_json.zip"}`,
		"unknown feed":     `{"feed":"nope"}`,
		"nothing at all":   `{}`,
		"both at once":     `{"feed":"all","url":"https://cricsheet.org/downloads/t20s_json.zip"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/ops/data/fetch", strings.NewReader(body))
			rec := httptest.NewRecorder()
			(&App{}).dataFetchHandler(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code, "body %s must be refused", body)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp["error"])
			assert.NotNil(t, resp["allowed_hosts"], "a refusal must say what would be accepted")
		})
	}
}

// TestFetchCommandComesFromTheRegistry guards the wiring the lane depends on. A
// command string that the registry does not recognise resolves to LaneCompute, which
// would silently put downloads back behind the training lock.
func TestFetchCommandComesFromTheRegistry(t *testing.T) {
	t.Parallel()
	step, ok := pipelinesvc.Steps().ByID("fetch")
	require.True(t, ok, "the fetch step must exist in the registry")
	assert.Equal(t, step.Command, fetchCommand)
	assert.Equal(t, pipelinesvc.LaneData, pipelinesvc.Steps().LaneForCommand(fetchCommand))
	assert.Equal(t, pipelinesvc.SurfaceData, step.EffectiveSurface())
}

// TestPipelineRunHandler_RefusesDataStepsWithSomewhereToGo: fetch is in the registry
// so its lane and label stay honest, but it is not a pipeline stage. A bare 400 would
// leave the caller guessing which endpoint to use instead.
func TestPipelineRunHandler_RefusesDataStepsWithSomewhereToGo(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/ops/pipeline/run/fetch", nil)
	req = mux.SetURLVars(req, map[string]string{"step": "fetch"})
	rec := httptest.NewRecorder()
	(&App{}).pipelineRunHandler(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "/ops/data/fetch")
}

// TestRequestedLanes covers the ?lane= parameter on Stop.
func TestRequestedLanes(t *testing.T) {
	t.Parallel()
	lanes, err := requestedLanes(httptest.NewRequest(http.MethodPost, "/ops/pipeline/stop", nil))
	require.NoError(t, err)
	assert.Nil(t, lanes, "no lane means every lane")

	lanes, err = requestedLanes(httptest.NewRequest(http.MethodPost, "/ops/pipeline/stop?lane=DATA", nil))
	require.NoError(t, err)
	assert.Equal(t, []pipelinesvc.Lane{pipelinesvc.LaneData}, lanes)

	_, err = requestedLanes(httptest.NewRequest(http.MethodPost, "/ops/pipeline/stop?lane=nonsense", nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compute", "the error must name the lanes that do exist")
}
