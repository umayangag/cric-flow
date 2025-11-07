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

func TestPredictBatting_Success_New(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict/batting" {
			t.Fatalf("path = %s, want /predict/batting", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		resp := []contracts.BattingPrediction{{RunsScored: 12, BallsFaced: 10}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
	ctx := context.Background()
	preds, err := c.PredictBatting(ctx, []contracts.BattingFeatures{{PlayerName: "A"}})
	if err != nil {
		t.Fatalf("PredictBatting error: %v", err)
	}
	if !called || len(preds) != 1 || preds[0].RunsScored != 12 {
		t.Fatalf("unexpected response: called=%v preds=%+v", called, preds)
	}
}

func TestPredictBowling_Success_New(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict/bowling" {
			t.Fatalf("path = %s, want /predict/bowling", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		resp := []contracts.BowlingPrediction{{WicketsTaken: 2}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
	ctx := context.Background()
	preds, err := c.PredictBowling(ctx, []contracts.BowlingFeatures{{PlayerName: "B"}})
	if err != nil {
		t.Fatalf("PredictBowling error: %v", err)
	}
	if !called || len(preds) != 1 || preds[0].WicketsTaken != 2 {
		t.Fatalf("unexpected response: called=%v preds=%+v", called, preds)
	}
}
