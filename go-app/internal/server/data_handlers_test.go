package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
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

// TestDataExtractHandler_RefusesAnUnsafeOrMissingArchive: the archive name comes from
// a request body, so it is checked rather than trusted — a caller asking to extract
// "../../etc/shadow" must be refused before anything opens it.
func TestDataExtractHandler_RefusesAnUnsafeOrMissingArchive(t *testing.T) {
	// Not parallel: t.Setenv points the dataset directory at a temp dir.
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)

	cases := map[string]string{
		"traversal":        `{"archive":"../../etc/shadow"}`,
		"nested path":      `{"archive":"sub/all_json.zip"}`,
		"backslash path":   `{"archive":"..\\all_json.zip"}`,
		"not an archive":   `{"archive":"matches.json"}`,
		"does not exist":   `{"archive":"missing.zip"}`,
		"nothing staged":   `{}`,
		"empty after trim": `{"archive":"   "}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/ops/data/extract", strings.NewReader(body))
			rec := httptest.NewRecorder()
			(&App{}).dataExtractHandler(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code, "body %s must be refused", body)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp["error"])
			assert.NotNil(t, resp["staging_dir"], "a refusal must say where archives are looked for")
		})
	}
}

func TestDataStagedHandler_ListsArchivesAndTheLiveManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)
	staging := dataset.StagingDir()
	require.NoError(t, os.MkdirAll(staging, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "all_json.zip"), []byte("PK"), 0o600))
	// A non-archive in staging must not be offered as one.
	require.NoError(t, os.WriteFile(filepath.Join(staging, "notes.txt"), []byte("x"), 0o600))

	rec := httptest.NewRecorder()
	(&App{}).dataStagedHandler(rec, httptest.NewRequest(http.MethodGet, "/ops/data/staged", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Archives   []dataacquire.StagedArchive `json:"archives"`
		StagingDir string                      `json:"staging_dir"`
		Live       *dataacquire.ExtractResult  `json:"live"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Archives, 1)
	assert.Equal(t, "all_json.zip", body.Archives[0].Filename)
	assert.Equal(t, staging, body.StagingDir)
	assert.Nil(t, body.Live, "a directory with no manifest has genuinely unknown provenance")
}

func TestExtractCommandComesFromTheRegistry(t *testing.T) {
	t.Parallel()
	step, ok := pipelinesvc.Steps().ByID("extract")
	require.True(t, ok, "the extract step must exist in the registry")
	assert.Equal(t, step.Command, extractCommand)
	assert.Equal(t, pipelinesvc.LaneData, pipelinesvc.Steps().LaneForCommand(extractCommand))
	assert.Equal(t, pipelinesvc.SurfaceData, step.EffectiveSurface())
	assert.NotEmpty(t, step.Prerequisite, "extract's staged-archive precondition must be stated")
}

func TestDataDatasetsHandler_ReportsTheLiveDigestEvenWithNoRows(t *testing.T) {
	// Not parallel: t.Setenv points the dataset directory at a temp dir.
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)
	db.SetDB(nil) // no registry available; the endpoint must still answer

	rec := httptest.NewRecorder()
	(&App{}).dataDatasetsHandler(rec, httptest.NewRequest(http.MethodGet, "/ops/data/datasets", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Datasets   []map[string]any `json:"datasets"`
		DatasetDir string           `json:"dataset_dir"`
		LiveSHA    string           `json:"live_sha256"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, dir, body.DatasetDir)
	assert.Empty(t, body.LiveSHA, "a directory with no manifest has no known live dataset")
}

// TestDataDatasetsHandler_ReadsTheLiveDigestFromTheManifest: which dataset is live is
// a property of the filesystem, so the handler must read it from disk rather than a
// column that an rsync could leave lying.
func TestDataDatasetsHandler_ReadsTheLiveDigestFromTheManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)
	db.SetDB(nil)

	manifest := dataacquire.ExtractResult{ArchiveSHA256: "live-digest", MatchFiles: 3}
	encoded, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dataacquire.ManifestPath(dir), encoded, 0o600))

	rec := httptest.NewRecorder()
	(&App{}).dataDatasetsHandler(rec, httptest.NewRequest(http.MethodGet, "/ops/data/datasets", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		LiveSHA string `json:"live_sha256"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "live-digest", body.LiveSHA)
}

func TestDatasetListLimit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, defaultDatasetListLimit, datasetListLimit(""), "absent means the default")
	assert.Equal(t, defaultDatasetListLimit, datasetListLimit("nonsense"))
	assert.Equal(t, defaultDatasetListLimit, datasetListLimit("0"), "zero is not a request for none")
	assert.Equal(t, defaultDatasetListLimit, datasetListLimit("-5"))
	assert.Equal(t, 10, datasetListLimit(" 10 "))
	assert.Equal(t, maxDatasetListLimit, datasetListLimit("9999"), "the cap bounds an unbounded request")
}

// TestParseTimestamp: the stamps come from results this process just produced, so a
// parse failure is a bug — but a wrong-by-seconds value beats one that renders as
// year 1 in every UI that touches it.
func TestParseTimestamp(t *testing.T) {
	t.Parallel()
	parsed := parseTimestamp("2026-08-26T12:00:00Z")
	assert.Equal(t, 2026, parsed.Year())

	fallback := parseTimestamp("not a timestamp")
	assert.WithinDuration(t, time.Now(), fallback, time.Minute)
	assert.NotEqual(t, 1, fallback.Year())
}
