package backtest_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/backtest"
)

func TestEmptySummaryAndProgressive(t *testing.T) {
	summary, progressive := svc.EmptySummaryAndProgressive()
	require.Equal(t, map[string]float64{"n": 0}, summary)
	require.Nil(t, progressive)
}

func TestComputeSummaryAndProgressive(t *testing.T) {
	items := []svc.AccuracyTrendItem{
		{Metrics: map[string]float64{"player_runs_mae": 10, "team_runs_mae": 5}},
		{Metrics: map[string]float64{"team_runs_mae": 7, "team_winner_accuracy": 1}},
		{Metrics: map[string]float64{"player_runs_mae": 20}},
	}
	summary, prog := svc.ComputeSummaryAndProgressive(items)

	require.Equal(t, float64(3), summary["n"])
	require.InDelta(t, 15.0, summary["player_runs_mae_avg"], 1e-9)
	require.InDelta(t, 6.0, summary["team_runs_mae_avg"], 1e-9)
	require.InDelta(t, 1.0, summary["team_winner_accuracy_avg"], 1e-9)
	require.Len(t, prog, 3)
	require.InDelta(t, 10.0, prog[0]["player_runs_mae_avg"], 1e-9)
	require.InDelta(t, 5.0, prog[0]["team_runs_mae_avg"], 1e-9)
	require.InDelta(t, 6.0, prog[1]["team_runs_mae_avg"], 1e-9)
	require.InDelta(t, 1.0, prog[1]["team_winner_accuracy_avg"], 1e-9)
}
