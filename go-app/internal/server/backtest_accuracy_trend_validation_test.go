package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"github.com/stretchr/testify/require"
)

func TestBacktestAccuracyTrendHandler_InvalidStartDate(t *testing.T) {
	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	// invalid start_date format (slashes)
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&start_date=2024/01/01",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestBacktestAccuracyTrendHandler_InvalidEndDate(t *testing.T) {
	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	// invalid end_date format (slashes)
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&end_date=2024/01/31",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestBacktestAccuracyTrendHandler_EndBeforeStart(t *testing.T) {
	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	// end before start should yield 400
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&start_date=2024-02-01&end_date=2024-01-01",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestBacktestAccuracyTrendHandler_EmptyResults(t *testing.T) {
	// Stub the listing seam to return empty slice
	orig := listPlayedMatchesByFilters
	defer func() { listPlayedMatchesByFilters = orig }()
	listPlayedMatchesByFilters = func(_ context.Context, _ string, _ string, _ string, _ time.Time, _ time.Time, _ string, _ int) ([]backtestCandidate, error) {
		return []backtestCandidate{}, nil
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&start_date=2024-01-01&end_date=2024-01-31",
		nil,
	)
	app.backtestAccuracyTrendHandler(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var payload accuracyTrendResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	require.Equal(t, 0, payload.Count)
	if len(payload.Results) != 0 {
		t.Fatalf("Results len = %d, want 0", len(payload.Results))
	}
	if n, ok := payload.Summary["n"]; !ok || n != 0 {
		t.Fatalf("Summary.n = %v, want 0", payload.Summary["n"])
	}
}
