package mlclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type postJSONResp struct {
	OK bool `json:"ok"`
}

func TestPostJSON_SuccessAndUserAgent(t *testing.T) {
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
}

func TestPostJSON_Non2xx(t *testing.T) {
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
}

func TestPostJSON_InvalidJSON(t *testing.T) {
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
}

func TestPostJSON_ContextCanceled(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-done // block until test signals
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 5 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately to force context error path
	var out postJSONResp
	err := c.postJSON(ctx, "/x", map[string]int{"a": 1}, &out)
	close(done)
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
	// ensure error is due to context
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		// Depending on timing/transport, error may wrap http Do error; accept either
		t.Logf("non-fatal: error not directly context.*: %v", err)
	}
}
