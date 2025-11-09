package mlclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
)

// Test helpers
func newTestClient(base string, httpClient *http.Client) *Client {
	return &Client{
		BaseURL:   base,
		HTTP:      httpClient,
		UserAgent: "mlclient-test/1.0",
		Timeout:   200 * time.Millisecond,
	}
}

type postJSONResp struct {
	OK bool `json:"ok"`
}

type badJSON struct{}

func (badJSON) MarshalJSON() ([]byte, error) { return nil, errors.New("marshal boom") }

// New() configuration tests (env default/override)
func TestClient_New(t *testing.T) {
	tests := []struct {
		name      string
		setEnv    bool
		envVal    string
		expectURL string
	}{
		{"defaults when env unset", false, "", "http://localhost:8000"},
		{"respects ML_BASE_URL env", true, "http://example.test:1234", "http://example.test:1234"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				os.Setenv("ML_BASE_URL", tc.envVal)
				t.Cleanup(func() { _ = os.Unsetenv("ML_BASE_URL") })
			} else {
				_ = os.Unsetenv("ML_BASE_URL")
			}
			c := New()
			if c.BaseURL != tc.expectURL {
				t.Fatalf("BaseURL = %q, want %q", c.BaseURL, tc.expectURL)
			}
			if c.HTTP == nil || c.Timeout <= 0 || c.HTTP.Timeout <= 0 {
				t.Fatalf("unexpected HTTP/Timeout configuration: Timeout=%v HTTP.Timeout=%v", c.Timeout, c.HTTP.Timeout)
			}
			if c.UserAgent == "" {
				t.Fatalf("UserAgent should be non-empty")
			}
		})
	}
}

// postJSON behavior tests (headers, errors, marshal, cancel)
func TestClient_postJSON(t *testing.T) {
	t.Run("success + user-agent", func(t *testing.T) {
		wantUA := "mlclient-test/1.0"
		seenUA := ""
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content-type = %s, want application/json", ct)
			}
			seenUA = r.Header.Get("User-Agent")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(postJSONResp{OK: true})
		}))
		defer srv.Close()

		c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), UserAgent: wantUA, Timeout: 2 * time.Second}
		ctx := context.Background()
		var out postJSONResp
		if err := c.postJSON(ctx, "/x", map[string]int{"a": 1}, &out); err != nil {
			t.Fatalf("postJSON error: %v", err)
		}
		if !out.OK {
			t.Fatalf("unexpected response: %+v", out)
		}
		if seenUA != wantUA {
			t.Fatalf("User-Agent seen %q, want %q", seenUA, wantUA)
		}
	})

	t.Run("non-2xx status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		}))
		defer srv.Close()

		c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
		ctx := context.Background()
		var out any
		if err := c.postJSON(ctx, "/x", map[string]bool{"ok": true}, &out); err == nil {
			t.Fatalf("expected error for non-2xx status")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		}))
		defer srv.Close()

		c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
		ctx := context.Background()
		var out any
		if err := c.postJSON(ctx, "/x", map[string]bool{"ok": true}, &out); err == nil {
			t.Fatalf("expected JSON decode error")
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		done := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			<-done
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 5 * time.Second}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out postJSONResp
		err := c.postJSON(ctx, "/x", map[string]int{"a": 1}, &out)
		close(done)
		if err == nil {
			t.Fatalf("expected context cancellation error")
		}
	})

	t.Run("marshal error", func(t *testing.T) {
		c := newTestClient("http://invalid", httptest.NewServer(nil).Client())
		var out any
		err := c.postJSON(context.Background(), "/unused", badJSON{}, &out)
		if err == nil {
			t.Fatalf("expected marshal error, got nil")
		}
	})
}

// PredictBatting behavior grouped
func TestClient_PredictBatting(t *testing.T) {
	tests := []struct {
		name       string
		serverFunc http.HandlerFunc
		expectErr  bool
		expectRuns float32
	}{
		{
			"success",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/predict/batting" || r.Method != http.MethodPost {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Fatalf("unexpected content-type: %s", ct)
				}
				_ = json.NewEncoder(w).Encode([]contracts.BattingPrediction{{RunsScored: 42}})
			}),
			false,
			42,
		},
		{
			"non-200",
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte(`{"error":"nope"}`))
			}),
			true,
			0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.serverFunc)
			defer srv.Close()
			c := newTestClient(srv.URL, srv.Client())
			_, err := c.PredictBatting(context.Background(), []contracts.BattingFeatures{{}})
			if tc.expectErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.expectErr {
				preds, _ := c.PredictBatting(context.Background(), []contracts.BattingFeatures{{}})
				if len(preds) != 1 || preds[0].RunsScored != tc.expectRuns {
					t.Fatalf("unexpected preds: %+v", preds)
				}
			}
		})
	}
}

// PredictBowling grouped
func TestClient_PredictBowling(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/predict/bowling" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode([]contracts.BowlingPrediction{{WicketsTaken: 3}})
		}))
		defer srv.Close()
		c := newTestClient(srv.URL, srv.Client())
		preds, err := c.PredictBowling(context.Background(), []contracts.BowlingFeatures{{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(preds) != 1 || preds[0].WicketsTaken != 3 {
			t.Fatalf("unexpected preds: %#v", preds)
		}
	})

	t.Run("bad JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		}))
		defer srv.Close()
		c := newTestClient(srv.URL, srv.Client())
		_, err := c.PredictBowling(context.Background(), []contracts.BowlingFeatures{{}})
		if err == nil {
			t.Fatalf("expected JSON decode error")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()
		hc := srv.Client()
		hc.Timeout = 50 * time.Millisecond
		c := newTestClient(srv.URL, hc)
		_, err := c.PredictBowling(context.Background(), []contracts.BowlingFeatures{{}})
		if err == nil {
			t.Fatalf("expected timeout error, got nil")
		}
	})

	t.Run("user-agent absent uses Go default", func(t *testing.T) {
		var sawUA string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawUA = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()
		c := newTestClient(srv.URL, srv.Client())
		c.UserAgent = "" // explicitly clear
		_, err := c.PredictBowling(context.Background(), []contracts.BowlingFeatures{{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sawUA != "Go-http-client/1.1" {
			t.Fatalf("expected default Go User-Agent, got %q", sawUA)
		}
	})
}
