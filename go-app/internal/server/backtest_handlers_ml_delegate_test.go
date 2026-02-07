package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// Test the backtest evaluate handler delegation path (use_ml=1)
func TestBacktestEvaluate_Handler_MLDelegation(t *testing.T) {
	// Save and restore seam
	orig := mlHistoricalBacktestFunc
	t.Cleanup(func() { mlHistoricalBacktestFunc = orig })

	cutoff := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)

	// Table-driven tests
	tests := []struct {
		name          string
		query         url.Values
		stub          func()
		wantStatus    int
		wantErrorCode string
		wantWinnerAcc *float64
		wantPlayerMAE *float64
	}{
		{
			name: "success maps payload",
			query: url.Values{
				"format":   []string{"T20"},
				"team1":    []string{"IND"},
				"team2":    []string{"AUS"},
				"mode":     []string{"evaluate"},
				"match_id": []string{"789"},
				"use_ml":   []string{"1"},
				"cutoff":   []string{cutoff.Format(time.RFC3339)},
			},
			stub: func() {
				mlHistoricalBacktestFunc = func(_ context.Context, _ time.Time, _ *int64, _ *HistoricalMatchFilters) (HistoricalBacktestResult, error) {
					one := 1.0
					trueVal := true
					return HistoricalBacktestResult{
						Players: []mlBacktestPlayerComparison{
							{
								PlayerID:     101,
								Predicted:    mlBacktestPlayerPoint{Runs: 20.0},
								Actual:       mlBacktestPlayerPoint{Runs: 18.0},
								AbsErrorRuns: 2.0,
							},
						},
						Match: mlBacktestMatchComparison{
							Predicted: mlBacktestMatchAgg{Runs: 160, Wickets: 6, Extras: 10, WinnerTeamCode: "IND"},
							Actual:    mlBacktestMatchAgg{Runs: 155, Wickets: 7, Extras: 12, WinnerTeamCode: "IND"},
						},
						Metrics: mlBacktestMetrics{
							MAERuns:       2.0,
							RMSERuns:      2.0,
							MAEWickets:    &one,
							WinnerCorrect: &trueVal,
						},
						ModelVersion: "v-test",
					}, nil
				}
			},
			wantStatus:    http.StatusOK,
			wantWinnerAcc: floatPtr(1.0),
			wantPlayerMAE: floatPtr(2.0),
		},
		{
			name: "missing cutoff when use_ml=1",
			query: url.Values{
				"format":   []string{"T20"},
				"team1":    []string{"IND"},
				"team2":    []string{"AUS"},
				"mode":     []string{"evaluate"},
				"match_id": []string{"789"},
				"use_ml":   []string{"1"},
			},
			stub:          func() {},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_PARAM",
		},
		{
			name: "bad cutoff format",
			query: url.Values{
				"format":   []string{"T20"},
				"team1":    []string{"IND"},
				"team2":    []string{"AUS"},
				"mode":     []string{"evaluate"},
				"match_id": []string{"789"},
				"use_ml":   []string{"1"},
				"cutoff":   []string{"not-a-time"},
			},
			stub:          func() {},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_PARAM",
		},
		{
			name: "ml seam returns error -> 500",
			query: url.Values{
				"format":   []string{"T20"},
				"team1":    []string{"IND"},
				"team2":    []string{"AUS"},
				"mode":     []string{"evaluate"},
				"match_id": []string{"789"},
				"use_ml":   []string{"1"},
				"cutoff":   []string{cutoff.Format(time.RFC3339)},
			},
			stub: func() {
				mlHistoricalBacktestFunc = func(_ context.Context, _ time.Time, _ *int64, _ *HistoricalMatchFilters) (HistoricalBacktestResult, error) {
					return HistoricalBacktestResult{}, errors.New("boom")
				}
			},
			wantStatus:    http.StatusInternalServerError,
			wantErrorCode: "INTERNAL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.stub()
			app := &App{}
			req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?"+tc.query.Encode(), nil)
			rr := httptest.NewRecorder()
			app.backtestMatchHandler(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			var body map[string]any
			_ = json.Unmarshal(rr.Body.Bytes(), &body)
			if tc.wantErrorCode != "" {
				// Expect error payload shape
				if body["code"] != tc.wantErrorCode {
					t.Fatalf("error code=%v want=%s body=%v", body["code"], tc.wantErrorCode, body)
				}
				return
			}
			// Success path assertions
			// Filters.delegated and model_version present
			filters, ok := body["filters"].(map[string]any)
			if !ok || filters["delegated"] != true || filters["model_version"] == nil {
				t.Fatalf("missing delegated/model_version in filters: %+v", body)
			}
			// Metrics include player_runs_mae and winner_accuracy
			metrics, ok := body["metrics"].(map[string]any)
			if !ok {
				t.Fatalf("missing metrics: %+v", body)
			}
			if tc.wantPlayerMAE != nil {
				if got, _ := metrics["player_runs_mae"].(float64); !floatApproxEqual(got, *tc.wantPlayerMAE) {
					t.Fatalf("player_runs_mae=%v want=%v", got, *tc.wantPlayerMAE)
				}
			}
			if tc.wantWinnerAcc != nil {
				if got, _ := metrics["winner_accuracy"].(float64); !floatApproxEqual(got, *tc.wantWinnerAcc) {
					t.Fatalf("winner_accuracy=%v want=%v", got, *tc.wantWinnerAcc)
				}
			}
			// Players array present
			if _, ok := body["players"].([]any); !ok {
				t.Fatalf("players not found or wrong type: %+v", body)
			}
			// Match aggregates present
			ma, ok := body["match_aggregates"].(map[string]any)
			if !ok || ma["predicted"] == nil || ma["actual"] == nil || ma["errors"] == nil {
				t.Fatalf("match_aggregates missing fields: %+v", body)
			}
		})
	}
}

func floatPtr(v float64) *float64 { return &v }

func floatApproxEqual(a, b float64) bool {
	const eps = 1e-9
	if a > b {
		return a-b < eps
	}
	return b-a < eps
}
