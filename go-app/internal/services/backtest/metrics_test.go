package backtest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
)

func TestComputeR2(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		sse     float64
		actuals []float64
		want    float64
	}{
		{"empty", 0, nil, 0},
		{"perfect", 0, []float64{1, 2, 3}, 1.0},
		{"zero_variance", 0, []float64{5, 5, 5}, 0},
		{"partial", 2.0, []float64{1, 2, 3}, 0.0},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tc.want, backtest.ComputeR2(tc.sse, tc.actuals), 1e-9)
		})
	}
}

func TestWinnerAccuracy(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		pred, actual string
		want         float64
	}{
		{"AUS", "AUS", 1},
		{"aus", "AUS", 1},
		{"AUS", "ENG", 0},
		{"", "AUS", 0},
		{"AUS", "", 0},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.pred+"_"+tc.actual, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, backtest.WinnerAccuracy(tc.pred, tc.actual))
		})
	}
}

func TestChooseBacktestMode(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		mode, matchID, want string
	}{
		{"", "", "select"},
		{"", "123", "evaluate"},
		{"evaluate", "", "evaluate"},
		{"select", "123", "select"},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.mode+"_"+tc.matchID, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, backtest.ChooseBacktestMode(tc.mode, tc.matchID))
		})
	}
}

func TestComputePlayerResultsAndMetrics(t *testing.T) {
	t.Parallel()
	squad := []int64{1, 2}
	preds := map[int64]backtest.PlayerPredictions{
		1: {Runs: 50, Wickets: 2, Economy: 6, Catches: 1, RunOuts: 0},
		2: {Runs: 30, Wickets: 0, Economy: 8, Catches: 0, RunOuts: 1},
	}
	actuals := map[int64]backtest.PlayerActuals{
		1: {Runs: 40, Wickets: 1, Economy: 5, Catches: 2, RunOuts: 0},
		2: {Runs: 35, Wickets: 1, Economy: 7, Catches: 0, RunOuts: 0},
	}
	players, metrics := backtest.ComputePlayerResultsAndMetrics(squad, preds, actuals)
	assert.Len(t, players, 2)
	assert.Contains(t, metrics, "player_runs_mae")
	assert.Contains(t, metrics, "player_runs_rmse")
	assert.Contains(t, metrics, "player_runs_r2")
	assert.Greater(t, metrics["player_runs_mae"], 0.0)
}

func TestFormatFromFilters(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", backtest.FormatFromFilters(nil))
	assert.Equal(t, "", backtest.FormatFromFilters(map[string]any{"team": "AUS"}))
	assert.Equal(t, "ODI", backtest.FormatFromFilters(map[string]any{"format": "odi"}))
	assert.Equal(t, "TEST", backtest.FormatFromFilters(map[string]any{"format": "test"}))
}
