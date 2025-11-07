package mlclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
)

func TestPredictBowling_UserAgentAbsentWhenEmpty(t *testing.T) {
	var sawUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	c.UserAgent = "" // explicitly clear
	ctx := context.Background()
	_, err := c.PredictBowling(ctx, []contracts.BowlingFeatures{{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When no User-Agent is set explicitly, Go's http client sends a default value.
	if sawUA != "Go-http-client/1.1" {
		t.Fatalf("expected default Go User-Agent, got %q", sawUA)
	}
}
