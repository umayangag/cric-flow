package mlclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
)

func newTestClient(base string, httpClient *http.Client) *Client {
	return &Client{
		BaseURL:   base,
		HTTP:      httpClient,
		UserAgent: "mlclient-test",
		Timeout:   time.Second,
	}
}

func TestPredictBatting_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict/batting" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("unexpected content-type: %s", ct)
		}
		_ = json.NewEncoder(w).Encode([]contracts.BattingPrediction{{RunsScored: 42}})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	ctx := context.Background()
	preds, err := c.PredictBatting(ctx, []contracts.BattingFeatures{{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preds) != 1 || preds[0].RunsScored != 42 {
		t.Fatalf("unexpected preds: %#v", preds)
	}
}

func TestPredictBowling_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict/bowling" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]contracts.BowlingPrediction{{WicketsTaken: 3}})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	ctx := context.Background()
	preds, err := c.PredictBowling(ctx, []contracts.BowlingFeatures{{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preds) != 1 || preds[0].WicketsTaken != 3 {
		t.Fatalf("unexpected preds: %#v", preds)
	}
}

func TestPredictBatting_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	ctx := context.Background()
	_, err := c.PredictBatting(ctx, []contracts.BattingFeatures{{}})
	if err == nil {
		t.Fatalf("expected error for non-200 status")
	}
}

func TestPredictBowling_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	ctx := context.Background()
	_, err := c.PredictBowling(ctx, []contracts.BowlingFeatures{{}})
	if err == nil {
		t.Fatalf("expected JSON decode error")
	}
}
