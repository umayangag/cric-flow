package mlclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
)

func TestPredictBatting_Non2xx_New(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
	_, err := c.PredictBatting(context.Background(), []contracts.BattingFeatures{{PlayerName: "A"}})
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
}

func TestPredictBowling_InvalidJSON_New(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
	_, err := c.PredictBowling(context.Background(), []contracts.BowlingFeatures{{PlayerName: "B"}})
	if err == nil {
		t.Fatalf("expected json decode error")
	}
}
