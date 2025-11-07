package mlclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func TestPredictWin_Success_New(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict-win" {
			t.Fatalf("path = %s, want /predict-win", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		resp := []predictor.PlayerPrediction{{PlayerName: "A", WinningProbability: 0.9}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
	ctx := context.Background()
	preds, err := c.PredictWin(ctx, []predictor.PlayerPrediction{{PlayerName: "A"}})
	if err != nil {
		t.Fatalf("PredictWin error: %v", err)
	}
	if !called || len(preds) != 1 || preds[0].PlayerName != "A" || preds[0].WinningProbability != 0.9 {
		t.Fatalf("unexpected response: called=%v preds=%+v", called, preds)
	}
}

func TestPredictWin_Non200_New(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
	_, err := c.PredictWin(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error for non-200")
	}
}

func TestPredictWin_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Timeout: 2 * time.Second}
	_, err := c.PredictWin(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected JSON decode error")
	}
}

func TestPredictWin_ContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	httpClient := &http.Client{Timeout: 50 * time.Millisecond}
	c := &Client{BaseURL: srv.URL, HTTP: httpClient, Timeout: 50 * time.Millisecond}
	_, err := c.PredictWin(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
