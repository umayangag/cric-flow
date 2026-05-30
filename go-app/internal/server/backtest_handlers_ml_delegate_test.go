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

	"github.com/stretchr/testify/require"
)

// Test the backtest evaluate handler delegation path (use_ml=1)
func TestBacktestEvaluate_Handler_MLDelegation(t *testing.T) {
	// Save and restore seam
	orig := mlHistoricalBacktestFunc
	t.Cleanup(func() { mlHistoricalBacktestFunc = orig })

	cutoff := time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC)

	// Table-driven tests
	testCases := []struct {
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

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			tc.stub()
			app := &App{}
			req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?"+tc.query.Encode(), nil)
			rr := httptest.NewRecorder()
			app.backtestMatchHandler(rr, req)
			require.Equal(t, tc.wantStatus, rr.Code, "body=%s", rr.Body.String())
			var body map[string]any
			_ = json.Unmarshal(rr.Body.Bytes(), &body)
			if tc.wantErrorCode != "" {
				// Expect error payload shape
				require.Equal(t, tc.wantErrorCode, body["code"])
				return
			}
			// Success path assertions
			// Filters.delegated and model_version present
			filters, ok := body["filters"].(map[string]any)
			require.True(t, ok, "filters missing")
			require.Equal(t, true, filters["delegated"])
			require.NotNil(t, filters["model_version"])
			// Metrics include player_runs_mae and winner_accuracy
			metrics, ok := body["metrics"].(map[string]any)
			require.True(t, ok)
			if tc.wantPlayerMAE != nil {
				got, _ := metrics["player_runs_mae"].(float64)
				require.InDelta(t, *tc.wantPlayerMAE, got, 1e-9)
			}
			if tc.wantWinnerAcc != nil {
				got, _ := metrics["winner_accuracy"].(float64)
				require.InDelta(t, *tc.wantWinnerAcc, got, 1e-9)
			}
			// Players array present
			_, ok = body["players"].([]any)
			require.True(t, ok, "players not found or wrong type")
			// Match aggregates present
			ma, ok := body["match_aggregates"].(map[string]any)
			require.True(t, ok, "match_aggregates missing")
			require.NotNil(t, ma["predicted"])
			require.NotNil(t, ma["actual"])
			require.NotNil(t, ma["errors"])
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
