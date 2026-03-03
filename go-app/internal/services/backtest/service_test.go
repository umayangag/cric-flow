package backtest

import "testing"

func almostEqual(a, b float64) bool {
	const eps = 1e-9
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < eps
}

func TestComputeSummaryAndProgressive(t *testing.T) {
	// Craft 3 items with partial metric presence
	items := []AccuracyTrendItem{
		{Metrics: map[string]float64{"player_runs_mae": 10, "team_runs_mae": 5}},
		{Metrics: map[string]float64{"team_runs_mae": 7, "team_winner_accuracy": 1}},
		{Metrics: map[string]float64{"player_runs_mae": 20}},
	}
	summary, prog := ComputeSummaryAndProgressive(items)

	// Summary checks
	if got, ok := summary["n"]; !ok || got != 3 {
		t.Fatalf("summary n expected 3, got %v", summary["n"])
	}
	if got := summary["player_runs_mae_avg"]; !almostEqual(got, 15) {
		t.Fatalf("player_runs_mae_avg got %v want 15", got)
	}
	if got := summary["team_runs_mae_avg"]; !almostEqual(got, 6) {
		t.Fatalf("team_runs_mae_avg got %v want 6", got)
	}
	if got := summary["team_winner_accuracy_avg"]; !almostEqual(got, 1) {
		t.Fatalf("team_winner_accuracy_avg got %v want 1", got)
	}

	// Progressive length equals items
	if len(prog) != 3 {
		t.Fatalf("progressive len got %d want 3", len(prog))
	}
	if got := prog[0]["player_runs_mae_avg"]; !almostEqual(got, 10) {
		t.Fatalf("prog0 player_runs_mae_avg got %v want 10", got)
	}
	if got := prog[0]["team_runs_mae_avg"]; !almostEqual(got, 5) {
		t.Fatalf("prog0 team_runs_mae_avg got %v want 5", got)
	}
	if got := prog[1]["team_runs_mae_avg"]; !almostEqual(got, 6) {
		t.Fatalf("prog1 team_runs_mae_avg got %v want 6", got)
	}
	if got := prog[1]["team_winner_accuracy_avg"]; !almostEqual(got, 1) {
		t.Fatalf("prog1 winner_accuracy got %v want 1", got)
	}
}
