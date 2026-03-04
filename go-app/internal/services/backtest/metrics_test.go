package backtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeR2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		sse     float64
		actuals []float64
		want    float64
	}{
		{"empty", 0, nil, 0},
		{"perfect", 0, []float64{1, 2, 3}, 1.0},
		{"zero_variance", 0, []float64{5, 5, 5}, 0},
		{"partial", 2.0, []float64{1, 2, 3}, 0.0}, // R² = 1 - SSE/SST = 1 - 2/2
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, ComputeR2(tt.sse, tt.actuals), 1e-9)
		})
	}
}

func TestWinnerAccuracy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pred, actual string
		want         float64
	}{
		{"AUS", "AUS", 1},
		{"aus", "AUS", 1},
		{"AUS", "ENG", 0},
		{"", "AUS", 0},
		{"AUS", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.pred+"_"+tt.actual, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, WinnerAccuracy(tt.pred, tt.actual))
		})
	}
}

func TestChooseBacktestMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode, matchID, want string
	}{
		{"", "", "select"},
		{"", "123", "evaluate"},
		{"evaluate", "", "evaluate"},
		{"select", "123", "select"},
	}
	for _, tt := range tests {
		t.Run(tt.mode+"_"+tt.matchID, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ChooseBacktestMode(tt.mode, tt.matchID))
		})
	}
}

func TestComputePlayerResultsAndMetrics(t *testing.T) {
	t.Parallel()
	squad := []int64{1, 2}
	preds := map[int64]PlayerPredictions{
		1: {Runs: 50, Wickets: 2, Economy: 6, Catches: 1, RunOuts: 0},
		2: {Runs: 30, Wickets: 0, Economy: 8, Catches: 0, RunOuts: 1},
	}
	actuals := map[int64]PlayerActuals{
		1: {Runs: 40, Wickets: 1, Economy: 5, Catches: 2, RunOuts: 0},
		2: {Runs: 35, Wickets: 1, Economy: 7, Catches: 0, RunOuts: 0},
	}
	players, metrics := ComputePlayerResultsAndMetrics(squad, preds, actuals)
	assert.Len(t, players, 2)
	assert.Contains(t, metrics, "player_runs_mae")
	assert.Contains(t, metrics, "player_runs_rmse")
	assert.Contains(t, metrics, "player_runs_r2")
	assert.Greater(t, metrics["player_runs_mae"], 0.0)
}

func TestFormatFromFilters(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", FormatFromFilters(nil))
	assert.Equal(t, "", FormatFromFilters(map[string]any{"team": "AUS"}))
	assert.Equal(t, "ODI", FormatFromFilters(map[string]any{"format": "odi"}))
	assert.Equal(t, "TEST", FormatFromFilters(map[string]any{"format": "test"}))
}
