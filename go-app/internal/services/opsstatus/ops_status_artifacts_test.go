package opsstatus_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/services/opsstatus"
)

// mlServiceStub answers /health and /artifacts/status and points ML_SERVICE_URL at itself.
func mlServiceStub(t *testing.T, health, artifacts func(http.ResponseWriter)) *http.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			health(w)
		case "/artifacts/status":
			artifacts(w)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("ML_SERVICE_URL", server.URL)
	client := server.Client()
	client.Timeout = 2 * time.Second
	return client
}

func writeRun(t *testing.T, root, runID string, manifest map[string]any) {
	t.Helper()
	dir := filepath.Join(root, opsstatus.RunsDirName, runID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	if manifest == nil {
		return
	}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o600))
}

func okHealth(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

// TestBuildArtifactsSection_MLServiceRunViewIsPassedThrough: ml-service is the only
// process that can say which run it loaded, so its answer is copied whole rather than
// re-derived here from filenames.
func TestBuildArtifactsSection_MLServiceRunViewIsPassedThrough(t *testing.T) {
	client := mlServiceStub(t, okHealth, func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"root":            "/models",
			"current_run":     "20260902T101500Z-ab12cd34",
			"loaded_run":      "20260902T101500Z-ab12cd34",
			"ratings_through": "2026-08-30",
			"ratings_stale":   false,
			"runs": []map[string]any{
				{"run_id": "20260902T101500Z-ab12cd34", "loaded": true, "current": true},
			},
		})
	})

	section, mlOK := opsstatus.BuildArtifactsSection(client, t.TempDir())

	require.True(t, mlOK)
	assert.Equal(t, "20260902T101500Z-ab12cd34", section["loaded_run"])
	assert.Equal(t, "2026-08-30", section["ratings_through"])
	assert.Len(t, section["runs"], 1)
}

// TestBuildArtifactsSection_RefusalReachesTheConsole: a run whose arrays this code
// cannot serve is refused rather than loaded (D-6), and the refusal is the thing an
// operator needs to see -- not an empty panel that reads as "nothing trained yet".
func TestBuildArtifactsSection_RefusalReachesTheConsole(t *testing.T) {
	client := mlServiceStub(t, okHealth, func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"current_run": "20260902T101500Z-ab12cd34",
			"loaded_run":  nil,
			"error":       "run 20260902T101500Z-ab12cd34: bat_pos_sum has width 1024, expected 13427",
			"runs":        []map[string]any{{"run_id": "20260902T101500Z-ab12cd34", "loaded": false}},
		})
	})

	section, _ := opsstatus.BuildArtifactsSection(client, t.TempDir())

	assert.Nil(t, section["loaded_run"])
	assert.Contains(t, section["error"], "expected 13427")
}

// TestBuildArtifactsSection_FallsBackToTheRunsOnDisk when ml-service cannot be asked.
// The scan can say what exists; it cannot say what is loaded, and does not claim to.
func TestBuildArtifactsSection_FallsBackToTheRunsOnDisk(t *testing.T) {
	client := mlServiceStub(t,
		func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) },
		func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) })
	root := t.TempDir()
	writeRun(t, root, "20260901T090000Z-11111111", map[string]any{
		"run_id": "20260901T090000Z-11111111", "cutoff": "2025-09-01", "ratings_through": "2025-08-31", "git_sha": "abc1234",
	})
	writeRun(t, root, "20260902T090000Z-22222222", map[string]any{
		"run_id": "20260902T090000Z-22222222", "cutoff": "2025-09-01", "ratings_through": "2025-08-31", "git_sha": "def5678",
	})

	section, mlOK := opsstatus.BuildArtifactsSection(client, root)

	require.False(t, mlOK)
	assert.Equal(t, false, section["reachable"])
	runs := section["runs"].([]map[string]any)
	require.Len(t, runs, 2)
	assert.Equal(t, "20260902T090000Z-22222222", runs[0]["run_id"], "newest first")
	assert.Equal(t, true, runs[0]["has_manifest"])
	assert.Equal(t, "2025-08-31", runs[0]["ratings_through"], "the run's data date is read off the listing (P2-2)")
	assert.Nil(t, runs[0]["refused"])
}

// TestBuildArtifactsSection_AManifestWithoutRatingsThroughIsListedAsRefused: a run written
// before the manifest recorded its date is listed with the reason it cannot be loaded,
// not with a blank where the date would be (P2-2, §8.7). The scan reports what
// ml-service will refuse; it does not read the date from anywhere else.
func TestBuildArtifactsSection_AManifestWithoutRatingsThroughIsListedAsRefused(t *testing.T) {
	client := mlServiceStub(t,
		func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) },
		func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) })
	root := t.TempDir()
	writeRun(t, root, "20260901T090000Z-11111111", map[string]any{
		"run_id": "20260901T090000Z-11111111", "cutoff": "2025-09-01", "git_sha": "abc1234",
	})

	section, _ := opsstatus.BuildArtifactsSection(client, root)

	runs := section["runs"].([]map[string]any)
	require.Len(t, runs, 1)
	assert.Equal(t, true, runs[0]["has_manifest"])
	assert.NotContains(t, runs[0], "ratings_through")
	assert.Contains(t, runs[0]["refused"], "20260901T090000Z-11111111")
	assert.Contains(t, runs[0]["refused"], "ratings_through")
}

// TestBuildArtifactsSection_ADirectoryWithNoManifestIsNotARun: H-16's question in its
// on-disk form. Artifacts nothing can attribute to a run are reported as exactly that.
func TestBuildArtifactsSection_ADirectoryWithNoManifestIsNotARun(t *testing.T) {
	client := mlServiceStub(t,
		func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) },
		func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) })
	root := t.TempDir()
	writeRun(t, root, "20260901T090000Z-11111111", nil)

	section, _ := opsstatus.BuildArtifactsSection(client, root)

	runs := section["runs"].([]map[string]any)
	require.Len(t, runs, 1)
	assert.Equal(t, false, runs[0]["has_manifest"])
	assert.Contains(t, runs[0]["refused"], "manifest.json", "the reason is on the wire, not only a flag")
}

// TestBuildArtifactsSection_NoRunsDirectoryIsAnEmptyList, not an error: a box that has
// never trained is a normal state.
func TestBuildArtifactsSection_NoRunsDirectoryIsAnEmptyList(t *testing.T) {
	client := mlServiceStub(t,
		func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) },
		func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) })

	section, _ := opsstatus.BuildArtifactsSection(client, filepath.Join(t.TempDir(), "nope"))

	assert.Empty(t, section["runs"])
}
