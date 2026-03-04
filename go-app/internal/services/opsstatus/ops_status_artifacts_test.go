package opsstatus

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper: simple HTTP client pointing at a test server
func newHTTPClientForServer(ts *httptest.Server) *http.Client {
	c := ts.Client()
	// Reduce potential flakiness
	c.Timeout = 2 * time.Second
	os.Setenv("ML_SERVICE_URL", ts.URL)
	return c
}

func TestBuildArtifactsSection_Table(t *testing.T) {
	type assertion func(t *testing.T, sec map[string]any, mlOK bool)

	// Case 1: ML /health ok, /artifacts/status returns detailed formats JSON
	tsDetail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "batting_model": true, "bowling_model": true})
		case "/artifacts/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"formats": map[string]any{
					"ODI": map[string]any{
						"batting": map[string]any{"exists": true, "loaded": true, "path": "/a.joblib"},
						"bowling": map[string]any{"exists": false},
					},
					"TEST": map[string]any{
						"batting": map[string]any{"exists": false},
						"bowling": map[string]any{"exists": true, "path": "/b.joblib"},
					},
					"T20I": map[string]any{
						"batting": map[string]any{"exists": false},
						"bowling": map[string]any{"exists": false},
					},
					"T20": map[string]any{
						"batting": map[string]any{"exists": false},
						"bowling": map[string]any{"exists": false},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer tsDetail.Close()

	// Case 2: ML unhealthy, FS fallback with files
	tsUnhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "fail"})
		case "/artifacts/status":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer tsUnhealthy.Close()

	tests := []struct {
		name   string
		setup  func(t *testing.T) (*http.Client, string) // client, fsRoot
		assert assertion
	}{
		{
			name: "ml_detail_endpoint_used",
			setup: func(t *testing.T) (*http.Client, string) {
				client := newHTTPClientForServer(tsDetail)
				return client, t.TempDir()
			},
			assert: func(t *testing.T, sec map[string]any, mlOK bool) {
				if !mlOK {
					t.Fatalf("expected mlOK=true")
				}
				fm := sec["formats"].(map[string]any)
				// Check values came from HTTP detail payload
				odi := fm["ODI"].(map[string]any)["batting"].(map[string]any)
				if ex := odi["exists"].(bool); !ex {
					t.Fatalf("ODI batting should exist from detail endpoint")
				}
				testBowl := fm["TEST"].(map[string]any)["bowling"].(map[string]any)
				if ex := testBowl["exists"].(bool); !ex {
					t.Fatalf("TEST bowling should exist from detail endpoint")
				}
			},
		},
		{
			name: "filesystem_fallback_when_ml_unhealthy",
			setup: func(t *testing.T) (*http.Client, string) {
				client := newHTTPClientForServer(tsUnhealthy)
				root := t.TempDir()
				// Per-format artifacts: scaler+model for batting/bowling
				writeFileWithLines(t, root, "batting_scaler_ODI.joblib", 1)
				writeFileWithLines(t, root, "batting_model_ODI.joblib", 1)
				writeFileWithLines(t, root, "bowling_scaler_ODI.joblib", 1)
				writeFileWithLines(t, root, "bowling_model_ODI.joblib", 1)
				writeFileWithLines(t, root, "bowling_scaler_TEST.joblib", 1)
				writeFileWithLines(t, root, "bowling_model_TEST.joblib", 1)
				return client, root
			},
			assert: func(t *testing.T, sec map[string]any, mlOK bool) {
				if mlOK {
					t.Fatalf("expected mlOK=false")
				}
				fm := sec["formats"].(map[string]any)
				odi := fm["ODI"].(map[string]any)
				if !odi["batting"].(map[string]any)["exists"].(bool) ||
					!odi["bowling"].(map[string]any)["exists"].(bool) {
					t.Fatalf("expected ODI batting & bowling discovered via FS fallback")
				}
				testFmt := fm["TEST"].(map[string]any)
				if testFmt["batting"].(map[string]any)["exists"].(bool) {
					t.Fatalf("unexpected TEST batting exists in FS fallback")
				}
				if !testFmt["bowling"].(map[string]any)["exists"].(bool) {
					t.Fatalf("expected TEST bowling discovered via FS fallback")
				}
			},
		},
		{
			name: "missing_dir_results_in_all_false",
			setup: func(t *testing.T) (*http.Client, string) {
				// No server needed; still set a benign client and URL
				ts := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }),
				)
				defer ts.Close()
				client := newHTTPClientForServer(ts)
				return client, filepath.Join(t.TempDir(), "does-not-exist")
			},
			assert: func(t *testing.T, sec map[string]any, _ bool) {
				fm := sec["formats"].(map[string]any)
				for _, f := range []string{"TEST", "ODI", "T20I", "T20"} {
					ent := fm[f].(map[string]any)
					if ent["batting"].(map[string]any)["exists"].(bool) ||
						ent["bowling"].(map[string]any)["exists"].(bool) {
						t.Fatalf("expected all false exists flags for %s", f)
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, root := tc.setup(t)
			sec, mlOK := BuildArtifactsSection(client, root)
			// marshal for potential debug
			_, _ = json.Marshal(sec)
			tc.assert(t, sec, mlOK)
		})
	}
}
