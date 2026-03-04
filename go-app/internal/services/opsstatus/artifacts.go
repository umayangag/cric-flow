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
		"unified": map[string]any{
			"batting":  map[string]any{"exists": false},
			"bowling":  map[string]any{"exists": false},
			"fielding": map[string]any{"exists": false},
			"extras":   map[string]any{"exists": false},
			"win":      map[string]any{"exists": false},
		},
	}
	fm := map[string]any{}
	for _, f := range artifactFormats {
		fm[f] = map[string]any{
			"batting":  map[string]any{"exists": false},
			"bowling":  map[string]any{"exists": false},
			"fielding": map[string]any{"exists": false},
			"extras":   map[string]any{"exists": false},
			"win":      map[string]any{"exists": false},
		}
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
				perFormatKeys := []string{"batting", "bowling", "fielding", "extras", "win"}
				for _, f := range artifactFormats {
					if fa, ok := formatsAny[f].(map[string]any); ok {
						tgt := fm[f].(map[string]any)
						for _, key := range perFormatKeys {
							if b, ok := fa[key].(map[string]any); ok {
								tgt[key] = b
							}
						}
						fm[f] = tgt
					}
				}
				section["formats"] = fm
				if legacyAny, ok := art["legacy"].(map[string]any); ok {
					section["unified"] = legacyAny
				}
				return section, mlHealth
			}
		}
	}

	entries, _ := os.ReadDir(fsRoot)
	for _, f := range artifactFormats {
		for _, kind := range []string{"batting", "bowling", "fielding", "extras", "win"} {
			if p, mod, ok := findPerFormatArtifact(entries, fsRoot, f, kind); ok {
				m := fm[f].(map[string]any)[kind].(map[string]any)
				m["exists"] = true
				m["path"] = p
				m["modified"] = mod.UTC().Format(time.RFC3339)
				fm[f].(map[string]any)[kind] = m
			}
		}
	}
	section["formats"] = fm
	if unif, ok := section["unified"].(map[string]any); ok {
		for _, kind := range []string{"batting", "bowling", "fielding", "extras", "win"} {
			if p, mod, ok := findLegacyArtifactByKind(entries, fsRoot, kind); ok {
				m := unif[kind].(map[string]any)
				m["exists"] = true
				m["path"] = p
				m["modified"] = mod.UTC().Format(time.RFC3339)
				unif[kind] = m
			}
		}
		section["unified"] = unif
	}
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
	var needScaler, modelPrefix string
	switch kind {
	case "batting":
		needScaler, modelPrefix = "batting_scaler_", "batting_model_"
	case "bowling":
		needScaler, modelPrefix = "bowling_scaler_", "bowling_model_"
	case "fielding":
		needScaler, modelPrefix = "fielding_scaler_", "fielding_model_"
	case "extras":
		modelPrefix = "extras_model_"
	case "win":
		modelPrefix = "win_model_"
	default:
		return "", time.Time{}, false
	}
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

func findLegacyArtifactByKind(entries []os.DirEntry, root string, kind string) (path string, mod time.Time, ok bool) {
	var needScaler, modelName string
	switch kind {
	case "batting":
		needScaler, modelName = "batting_scaler.joblib", "batting_model.joblib"
	case "bowling":
		needScaler, modelName = "bowling_scaler.joblib", "bowling_model.joblib"
	case "fielding":
		needScaler, modelName = "fielding_scaler.joblib", "fielding_model.joblib"
	case "extras":
		modelName = "extras_model.joblib"
	case "win":
		modelName = "win_model.joblib"
	default:
		return "", time.Time{}, false
	}
	hasScaler := needScaler == ""
	hasModel := false
	var modelPath string
	var modelMod time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if needScaler != "" && lower == strings.ToLower(needScaler) {
			hasScaler = true
			continue
		}
		if lower == strings.ToLower(modelName) {
			full := filepath.Join(root, name)
			info, err := os.Stat(full)
			if err != nil || info.IsDir() {
				continue
			}
			modelPath = full
			modelMod = info.ModTime()
			hasModel = true
			break
		}
	}
	if hasScaler && hasModel {
		return modelPath, modelMod, true
	}
	return "", time.Time{}, false
}

type httpError struct{ code int }

func (e *httpError) Error() string { return "http status " + strconv.Itoa(e.code) }
