package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// formats supported for reporting
var artifactFormats = []string{"TEST", "ODI", "T20I", "T20"}

// artifactsFallbackRoot returns the filesystem root for the artifacts fallback scan
// when the ML service is unreachable. GO_APP_ARTIFACTS_ROOT overrides the default.
func artifactsFallbackRoot() string {
	if p := strings.TrimSpace(os.Getenv("GO_APP_ARTIFACTS_ROOT")); p != "" {
		return p
	}
	return filepath.Join("output", "ml-service")
}

// buildArtifactsSection probes the ML service (if available) and/or filesystem to
// construct the artifacts section for /ops/status. It also returns the mlHealth bool.
func buildArtifactsSection(client *http.Client, fsRoot string) (section map[string]any, mlHealth bool) {
	base := os.Getenv("ML_SERVICE_URL")
	if strings.TrimSpace(base) == "" {
		base = "http://localhost:8000"
	}
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}

	// default scaffold
	section = map[string]any{
		"root":    fsRoot,
		"formats": map[string]any{},
	}
	fm := map[string]any{}
	for _, f := range artifactFormats {
		fm[f] = map[string]any{
			"batting": map[string]any{"exists": false},
			"bowling": map[string]any{"exists": false},
		}
	}
	section["formats"] = fm

	// Probe ML /health first
	type healthResp struct {
		Status       string `json:"status"`
		BattingModel bool   `json:"batting_model"`
		BowlingModel bool   `json:"bowling_model"`
	}
	if h, err := httpGetJSON[healthResp](client, base+"/health"); err == nil && strings.EqualFold(h.Status, "ok") {
		mlHealth = true
	}

	// Try detailed artifacts endpoint; if 200 OK and parseable, use it
	// Expected shape (lenient): { formats: { FMT: { batting: {exists, loaded, modified, path}, bowling: {...} } } }
	var art map[string]any
	if resp, code, err := httpGetRaw(client, base+"/artifacts/status"); err == nil && code >= 200 && code < 300 {
		if err := json.Unmarshal(resp, &art); err == nil {
			if formatsAny, ok := art["formats"].(map[string]any); ok {
				for _, f := range artifactFormats {
					if fa, ok := formatsAny[f].(map[string]any); ok {
						tgt := fm[f].(map[string]any)
						if b, ok := fa["batting"].(map[string]any); ok {
							tgt["batting"] = b
						}
						if b, ok := fa["bowling"].(map[string]any); ok {
							tgt["bowling"] = b
						}
						fm[f] = tgt
					}
				}
				section["formats"] = fm
				return section, mlHealth
			}
		}
		// If parse failed, fall through to FS
	}

	// Filesystem fallback
	for _, f := range artifactFormats {
		// batting
		if p, mod, ok := findArtifact(fsRoot, f, true); ok {
			b := fm[f].(map[string]any)["batting"].(map[string]any)
			b["exists"] = true
			b["path"] = p
			b["modified"] = mod.UTC().Format(time.RFC3339)
			fm[f].(map[string]any)["batting"] = b
		}
		// bowling
		if p, mod, ok := findArtifact(fsRoot, f, false); ok {
			b := fm[f].(map[string]any)["bowling"].(map[string]any)
			b["exists"] = true
			b["path"] = p
			b["modified"] = mod.UTC().Format(time.RFC3339)
			fm[f].(map[string]any)["bowling"] = b
		}
	}
	section["formats"] = fm
	return section, mlHealth
}

// httpGetJSON performs a GET and unmarshals JSON into T.
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

// httpGetRaw returns body bytes, status code, error
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

// findArtifact tries to find a single artifact file for a format and kind (batting=true, bowling=false).
func findArtifact(root, format string, batting bool) (path string, mod time.Time, ok bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", time.Time{}, false
	}
	tokenKind := "bat"
	if !batting {
		tokenKind = "bowl"
	}
	fmtLower := strings.ToLower(format)
	bestName := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".joblib") {
			continue
		}
		if strings.Contains(lower, tokenKind) && strings.Contains(lower, fmtLower) {
			// prefer exact batting_FORMAT.joblib over generic patterns
			if batting {
				if strings.HasPrefix(lower, "batting_") && strings.Contains(lower, fmtLower) {
					bestName = name
					break
				}
			} else {
				if strings.HasPrefix(lower, "bowling_") && strings.Contains(lower, fmtLower) {
					bestName = name
					break
				}
			}
			if bestName == "" {
				bestName = name
			}
		}
	}
	if bestName == "" {
		return "", time.Time{}, false
	}
	full := filepath.Join(root, bestName)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return "", time.Time{}, false
	}
	return full, info.ModTime(), true
}

type httpError struct{ code int }

func (e *httpError) Error() string { return "http status " + strconv.Itoa(e.code) }
