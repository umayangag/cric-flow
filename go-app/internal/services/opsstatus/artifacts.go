package opsstatus

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// RunsDirName is the directory under the artifacts root that holds one subdirectory per
// training run. It mirrors ml.xi.runs.RUNS_DIRNAME; the fallback scan below is the only
// reason go-app needs to know it.
const RunsDirName = "runs"

// manifestName is the file that makes a directory a run (H-16). A directory of joblib
// files without one is not a run and is not reported as one.
const manifestName = "manifest.json"

// ArtifactsFallbackRoot returns the filesystem root for the artifacts fallback scan
// when the ML service is unreachable. GO_APP_ARTIFACTS_ROOT overrides the default.
func ArtifactsFallbackRoot() string {
	if p := strings.TrimSpace(os.Getenv("GO_APP_ARTIFACTS_ROOT")); p != "" {
		return p
	}
	return filepath.Join("output", "ml-service")
}

// BuildArtifactsSection reports which run ml-service is serving and which runs exist.
//
// It reports runs rather than a formats-by-model-kind matrix because a run is what an
// artifact belongs to now (H-16): the question "is the model current?" is answered by
// which run `current` points at and whether that is the run the process loaded, not by
// six per-format files each of which could come from a different training session.
//
// ml-service is the authority — it is the process that loaded something — and its
// /artifacts/status answer is copied through whole, including any refusal it reports
// (D-6: a run whose arrays this code cannot serve is refused, not loaded). The
// filesystem scan is the fallback for an unreachable service and can only say what is
// on disk, never what is loaded.
func BuildArtifactsSection(client *http.Client, fsRoot string) (section map[string]any, mlHealth bool) {
	base := os.Getenv("ML_SERVICE_URL")
	if strings.TrimSpace(base) == "" {
		base = config.ServerMLBaseURLFallback(config.Load())
	}
	if client == nil {
		sec := config.ServerArtifactsTimeoutSec(config.Load())
		client = &http.Client{Timeout: time.Duration(sec) * time.Second}
	}

	type healthResp struct {
		Status string `json:"status"`
	}
	if h, err := httpGetJSON[healthResp](client, base+"/health"); err == nil && strings.EqualFold(h.Status, "ok") {
		mlHealth = true
	}

	if resp, code, err := httpGetRaw(client, base+"/artifacts/status"); err == nil && code >= 200 && code < 300 {
		var art map[string]any
		if err := json.Unmarshal(resp, &art); err == nil && art["runs"] != nil {
			return art, mlHealth
		}
	}

	return scanRuns(fsRoot), mlHealth
}

// scanRuns lists the runs on disk when ml-service cannot be asked. It reports each run's
// manifest and says so when a directory has none, which is the on-disk half of D-6's
// question: a directory of artifacts nothing can attribute to a run.
func scanRuns(fsRoot string) map[string]any {
	section := map[string]any{"root": fsRoot, "reachable": false, "runs": []map[string]any{}}
	entries, err := os.ReadDir(filepath.Join(fsRoot, RunsDirName))
	if err != nil {
		return section
	}
	runs := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		runs = append(runs, describeRunDir(filepath.Join(fsRoot, RunsDirName, e.Name()), e.Name()))
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i]["run_id"].(string) > runs[j]["run_id"].(string)
	})
	section["runs"] = runs
	return section
}

// describeRunDir reads one run directory's manifest, reporting the absence of one rather
// than guessing at the contents.
//
// `ratings_through` is copied through like the other manifest fields (P2-2), and a
// manifest that predates it is reported with `refused` set rather than a blank: the
// scan cannot load anything, but it can say what ml-service will refuse when it is
// asked, so the fallback listing answers the same question the live one does (§8.7).
func describeRunDir(dir, name string) map[string]any {
	out := map[string]any{"run_id": name, "path": dir, "has_manifest": false, "refused": nil}
	raw, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		out["refused"] = name + " has no " + manifestName + ", so nothing says which run produced its artifacts (H-16)"
		return out
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		out["refused"] = name + ": " + manifestName + " is not readable as a run manifest: " + err.Error()
		return out
	}
	out["has_manifest"] = true
	for _, key := range []string{"run_id", "created_at", "cutoff", "ratings_through", "git_sha", "dataset_sha", "formats"} {
		if v, ok := manifest[key]; ok {
			out[key] = v
		}
	}
	if s, ok := manifest["ratings_through"].(string); !ok || strings.TrimSpace(s) == "" {
		out["refused"] = "run " + name + ": " + manifestName + " carries no ratings_through, so the date its data " +
			"runs through is not written down; it was written before the field existed and cannot be loaded"
	}
	// The second refusal ml-service applies (EVAL-12), mirrored for the same reason as the
	// first: this scan cannot load anything, but the fallback listing has to answer the
	// same question the live one does, and a run reported as loadable that ml-service will
	// refuse is worse than no listing at all.
	if digest, ok := manifest["dataset_digest"].(map[string]any); !ok || len(digest) == 0 {
		out["refused"] = "run " + name + ": " + manifestName + " carries no dataset_digest, so nothing says what " +
			"its dataset_sha is a digest of; it was written before the digest could see a squad or a delivery " +
			"(EVAL-12) and cannot be loaded"
	}
	return out
}

func httpGetJSON[T any](client *http.Client, url string) (T, error) {
	var zero T
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, &httpError{code: resp.StatusCode}
	}
	dec := json.NewDecoder(resp.Body)
	var out T
	if err := dec.Decode(&out); err != nil {
		return zero, err
	}
	return out, nil
}

func httpGetRaw(client *http.Client, url string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
}

type httpError struct{ code int }

func (e *httpError) Error() string { return "http status " + strconv.Itoa(e.code) }
