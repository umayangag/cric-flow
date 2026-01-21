package mlclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// dummyOut is used to decode JSON into a no-op type
// when we do not actually care about the content.
type dummyOut struct {
	OK bool `json:"ok"`
}

func TestDoJSON_Non2xx(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{name: "400", status: http.StatusBadRequest},
		{name: "500", status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		// capture tc for closure
		status := tc.status
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("oops"))
			}))
			defer srv.Close()

			client := DefaultClient(2 * time.Second)
			req, err := newRequest(context.Background(), http.MethodGet, srv.URL, nil, "test-agent")
			if err != nil {
				t.Fatalf("newRequest err: %v", err)
			}
			var out dummyOut
			resp, derr := doJSON(client, req, &out)
			if derr == nil {
				t.Fatalf("expected error for non-2xx, got nil")
			}
			if resp == nil {
				t.Fatalf("expected non-nil response on non-2xx")
			}
			if !strings.HasPrefix(derr.Error(), "ml-service status:") {
				t.Fatalf("error should start with 'ml-service status:', got %v", derr)
			}
		})
	}
}

func TestDoJSON_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{invalid-json"))
	}))
	defer srv.Close()

	client := DefaultClient(2 * time.Second)
	req, err := newRequest(context.Background(), http.MethodGet, srv.URL, nil, "test-agent")
	if err != nil {
		t.Fatalf("newRequest err: %v", err)
	}
	var out dummyOut
	resp, derr := doJSON(client, req, &out)
	if resp == nil {
		t.Fatalf("expected response, got nil")
	}
	if derr == nil {
		t.Fatalf("expected decode error, got nil")
	}
}

func TestDefaultClient_Timeout(t *testing.T) {
	// Server sleeps longer than client timeout
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	client := DefaultClient(50 * time.Millisecond)
	req, err := newRequest(context.Background(), http.MethodGet, srv.URL, nil, "")
	if err != nil {
		t.Fatalf("newRequest err: %v", err)
	}
	var out dummyOut
	_, derr := doJSON(client, req, &out)
	if derr == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	// Accept either client timeout or context deadline exceeded wording
	if !strings.Contains(derr.Error(), "Client.Timeout") && !errors.Is(derr, context.DeadlineExceeded) {
		// We cannot reliably unwrap here without importing net/http internals; do a substring fallback
		if !strings.Contains(derr.Error(), "timeout") {
			t.Fatalf("expected timeout-related error, got: %v", derr)
		}
	}
}

func TestNewRequest_Headers(t *testing.T) {
	// with User-Agent
	req1, err := newRequest(context.Background(), http.MethodPost, "http://example", map[string]int{"a": 1}, "ua-1")
	if err != nil {
		t.Fatalf("newRequest err: %v", err)
	}
	if ct := req1.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type expected application/json, got %q", ct)
	}
	if ua := req1.Header.Get("User-Agent"); ua != "ua-1" {
		t.Fatalf("user-agent expected 'ua-1', got %q", ua)
	}
	// without User-Agent
	req2, err := newRequest(context.Background(), http.MethodPost, "http://example", map[string]int{"a": 1}, "")
	if err != nil {
		t.Fatalf("newRequest err: %v", err)
	}
	if ua := req2.Header.Get("User-Agent"); ua != "" {
		t.Fatalf("user-agent expected empty, got %q", ua)
	}
}
