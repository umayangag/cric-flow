package mlclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
)

// Verifies that postJSON (indirectly via PredictBatting) forwards the configured User-Agent header.
func TestPredictBatting_SetsUserAgentHeader(t *testing.T) {
	var sawUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	// Use the helper to ensure UserAgent is set to a known value.
	c := newTestClient(srv.URL, srv.Client())
	ctx := context.Background()
	_, err := c.PredictBatting(ctx, []contracts.BattingFeatures{{}})
	if err != nil {
		// Even if decode fails (it won't here), we still want to assert header capture.
		t.Fatalf("unexpected error: %v", err)
	}
	if sawUA != c.UserAgent {
		t.Fatalf("User-Agent header mismatch: got %q want %q", sawUA, c.UserAgent)
	}
}

// Ensures client timeout/cancel surfaces as an error path from postJSON callers.
func TestPredictBowling_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate a slow server exceeding client timeout
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	// Client with very short timeout to force deadline exceeded
	httpClient := srv.Client()
	httpClient.Timeout = 50 * time.Millisecond
	c := newTestClient(srv.URL, httpClient)

	ctx := context.Background()
	_, err := c.PredictBowling(ctx, []contracts.BowlingFeatures{{}})
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}
