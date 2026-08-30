package opsstatus

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	formatsPkg "github.com/umayangag/cric-flow/go-app/internal/formats"
)

// formats supported for reporting (canonical order from internal/formats)
var artifactFormats = formatsPkg.CanonicalCodes()

// artifactKind names one family of per-format model artifacts and how it is named on disk.
// A kind with a scaler needs both files before it counts as present: the model alone cannot
// be loaded for inference.
type artifactKind struct {
	name         string
	modelPrefix  string
	scalerPrefix string // empty when the kind trains without one
}

// artifactKinds is the single list this package reports on: the models `make train-models`
// produces. Everything below derives from it, so a model kind cannot be trained by the
// pipeline and stay invisible in /ops/status -- which is what happened to innings.
var artifactKinds = []artifactKind{
	{name: "batting", modelPrefix: "batting_model_", scalerPrefix: "batting_scaler_"},
	{name: "bowling", modelPrefix: "bowling_model_", scalerPrefix: "bowling_scaler_"},
	{name: "fielding", modelPrefix: "fielding_model_", scalerPrefix: "fielding_scaler_"},
	{name: "extras", modelPrefix: "extras_model_"},
	{name: "win", modelPrefix: "win_model_"},
	{name: "innings", modelPrefix: "innings_model_", scalerPrefix: "innings_scaler_"},
}

func artifactKindByName(name string) (artifactKind, bool) {
	for _, k := range artifactKinds {
		if k.name == name {
			return k, true
		}
	}
	return artifactKind{}, false
}

// ArtifactsFallbackRoot returns the filesystem root for the artifacts fallback scan
// when the ML service is unreachable. GO_APP_ARTIFACTS_ROOT overrides the default.
func ArtifactsFallbackRoot() string {
	if p := strings.TrimSpace(os.Getenv("GO_APP_ARTIFACTS_ROOT")); p != "" {
		return p
	}
	return filepath.Join("output", "ml-service")
}

// BuildArtifactsSection probes the ML service (if available) and/or filesystem to
// construct the artifacts section for /ops/status. It also returns the mlHealth bool.
func BuildArtifactsSection(client *http.Client, fsRoot string) (section map[string]any, mlHealth bool) {
	base := os.Getenv("ML_SERVICE_URL")
	if strings.TrimSpace(base) == "" {
		base = config.ServerMLBaseURLFallback(config.Load())
	}
	if client == nil {
		sec := config.ServerArtifactsTimeoutSec(config.Load())
		client = &http.Client{Timeout: time.Duration(sec) * time.Second}
	}

	section = map[string]any{
		"root":    fsRoot,
		"formats": map[string]any{},
	}
	fm := map[string]any{}
	for _, f := range artifactFormats {
		row := map[string]any{}
		for _, kind := range artifactKinds {
			row[kind.name] = map[string]any{"exists": false}
		}
		fm[f] = row
	}
	section["formats"] = fm

	type healthResp struct {
		Status       string `json:"status"`
		BattingModel bool   `json:"batting_model"`
		BowlingModel bool   `json:"bowling_model"`
	}
	if h, err := httpGetJSON[healthResp](client, base+"/health"); err == nil && strings.EqualFold(h.Status, "ok") {
		mlHealth = true
	}

	var art map[string]any
	if resp, code, err := httpGetRaw(client, base+"/artifacts/status"); err == nil && code >= 200 && code < 300 {
		if err := json.Unmarshal(resp, &art); err == nil {
			if formatsAny, ok := art["formats"].(map[string]any); ok {
				for _, f := range artifactFormats {
					if fa, ok := formatsAny[f].(map[string]any); ok {
						tgt := fm[f].(map[string]any)
						// Copied whole, so whatever the ML service reports per cell -- including
						// its `loaded` and `stale` verdicts -- reaches the console unflattened.
						for _, kind := range artifactKinds {
							if b, ok := fa[kind.name].(map[string]any); ok {
								tgt[kind.name] = b
							}
						}
						fm[f] = tgt
					}
				}
				section["formats"] = fm
				return section, mlHealth
			}
		}
	}

	entries, _ := os.ReadDir(fsRoot)
	for _, f := range artifactFormats {
		for _, kind := range artifactKinds {
			if p, mod, ok := findPerFormatArtifact(entries, fsRoot, f, kind.name); ok {
				m := fm[f].(map[string]any)[kind.name].(map[string]any)
				m["exists"] = true
				m["path"] = p
				m["modified"] = mod.UTC().Format(time.RFC3339)
				fm[f].(map[string]any)[kind.name] = m
			}
		}
	}
	section["formats"] = fm
	return section, mlHealth
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

func findPerFormatArtifact(
	entries []os.DirEntry,
	root string,
	format string,
	kind string,
) (path string, mod time.Time, ok bool) {
	ak, known := artifactKindByName(kind)
	if !known {
		return "", time.Time{}, false
	}
	needScaler, modelPrefix := ak.scalerPrefix, ak.modelPrefix
	modelSuffix := format + ".joblib"
	scalerSuffix := format + ".joblib"
	hasScaler := needScaler == ""
	var modelPath string
	var modelMod time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".joblib") {
			continue
		}
		if needScaler != "" && strings.HasPrefix(lower, strings.ToLower(needScaler)) &&
			strings.HasSuffix(lower, strings.ToLower(scalerSuffix)) {
			hasScaler = true
			continue
		}
		if strings.HasPrefix(lower, strings.ToLower(modelPrefix)) &&
			strings.HasSuffix(lower, strings.ToLower(modelSuffix)) {
			full := filepath.Join(root, name)
			info, err := os.Stat(full)
			if err != nil || info.IsDir() {
				continue
			}
			modelPath = full
			modelMod = info.ModTime()
		}
	}
	if hasScaler && modelPath != "" {
		return modelPath, modelMod, true
	}
	return "", time.Time{}, false
}

type httpError struct{ code int }

func (e *httpError) Error() string { return "http status " + strconv.Itoa(e.code) }
