package selection

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/predictor"
)

func TestSelectTopWithMinBowlers(t *testing.T) {
	preds := []predictor.PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.90, Deliveries: 0, Econ: 0},    // batter
		{PlayerName: "B", WinningProbability: 0.85, Deliveries: 0, Econ: 0},    // batter
		{PlayerName: "C", WinningProbability: 0.80, Deliveries: 24, Econ: 6.5}, // bowler
		{PlayerName: "D", WinningProbability: 0.70, Deliveries: 0, Econ: 0},    // batter
		{PlayerName: "E", WinningProbability: 0.65, Deliveries: 30, Econ: 7.0}, // bowler
	}

	// Case: teamSize 3, need at least 1 bowler -> should be satisfied by initial top-3 after sorting
	sel, err := selectTopWithMinBowlers(append([]predictor.PlayerPrediction{}, preds...), 3, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sel) != 3 {
		t.Fatalf("expected 3 selected, got %d", len(sel))
	}
	bowlers := 0
	for _, p := range sel {
		if p.Deliveries > 0 || p.Econ > 0 {
			bowlers++
		}
	}
	if bowlers < 1 {
		t.Fatalf("expected at least 1 bowler, got %d", bowlers)
	}

	// Case: teamSize 3, require 2 bowlers -> should swap one batter with next bowler
	sel2, err := selectTopWithMinBowlers(append([]predictor.PlayerPrediction{}, preds...), 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sel2) != 3 {
		t.Fatalf("expected 3 selected, got %d", len(sel2))
	}
	bowlers = 0
	for _, p := range sel2 {
		if p.Deliveries > 0 || p.Econ > 0 {
			bowlers++
		}
	}
	if bowlers < 2 {
		t.Fatalf("expected at least 2 bowlers after swap, got %d", bowlers)
	}

	// Case: pool too small
	_, err = selectTopWithMinBowlers(append([]predictor.PlayerPrediction{}, preds...), 10, 1)
	if err == nil {
		t.Fatalf("expected error for small pool, got nil")
	}

	// Case: minBowlers 0 - no constraint, just top N
	sel3, err := selectTopWithMinBowlers(append([]predictor.PlayerPrediction{}, preds...), 3, 0)
	require.NoError(t, err)
	require.Len(t, sel3, 3)

	// Case: all top N already bowlers - no swap needed
	allBowlers := []predictor.PlayerPrediction{
		{PlayerName: "B1", WinningProbability: 0.9, Deliveries: 24, Econ: 6},
		{PlayerName: "B2", WinningProbability: 0.85, Deliveries: 30, Econ: 7},
		{PlayerName: "B3", WinningProbability: 0.8, Deliveries: 24, Econ: 5},
		{PlayerName: "B4", WinningProbability: 0.7, Deliveries: 18, Econ: 8},
	}
	sel4, err := selectTopWithMinBowlers(allBowlers, 3, 2)
	require.NoError(t, err)
	require.Len(t, sel4, 3)
}

func TestComputeAverageWinProbability(t *testing.T) {
	// Empty slice -> 0
	if got := computeAverageWinProbability(nil); got != 0 {
		t.Fatalf("avg empty got %v want 0", got)
	}
	ps := []predictor.PlayerPrediction{
		{WinningProbability: 0.5},
		{WinningProbability: 0.7},
		{WinningProbability: 0.9},
	}
	want := (0.5 + 0.7 + 0.9) / 3.0
	if got := computeAverageWinProbability(ps); math.Abs(got-want) > 1e-9 {
		t.Fatalf("avg got %v want ~%v", got, want)
	}
}

func TestParseF64(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"", 0},
		{"3.14", 3.14},
		{"0", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		if got := parseF64(tt.in); got != tt.want {
			t.Errorf("parseF64(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPrevSeasonName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2024", "2023"},
		{"2019", "2018"},
		{"invalid", "invalid"},
	}
	for _, tt := range tests {
		if got := prevSeasonName(tt.in); got != tt.want {
			t.Errorf("prevSeasonName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseSeasonInt(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"2024", 2024},
		{" 2019 ", 2019},
		{"x", 0},
	}
	for _, tt := range tests {
		if got := parseSeasonInt(tt.in); got != tt.want {
			t.Errorf("parseSeasonInt(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestNz64(t *testing.T) {
	if got := nz64(struct {
		Int64 int64
		Valid bool
	}{10, true}); got != 10 {
		t.Errorf("nz64(valid) = %d, want 10", got)
	}
	if got := nz64(struct {
		Int64 int64
		Valid bool
	}{10, false}); got != 0 {
		t.Errorf("nz64(invalid) = %d, want 0", got)
	}
}

func TestF32(t *testing.T) {
	if got := f32(3.14); got != 3.14 {
		t.Errorf("f32(3.14) = %v, want 3.14", got)
	}
}

func TestParsePlayersFromCSV(t *testing.T) {
	t.Parallel()

	t.Run("empty rows", func(t *testing.T) {
		header := []string{"player_name", "runs_scored"}
		got := parsePlayersFromCSV(header, nil)
		require.Empty(t, got)
	})

	t.Run("maps known columns", func(t *testing.T) {
		header := []string{
			"player_name",
			"runs_scored",
			"balls_faced",
			"deliveries",
			"wickets_taken",
			"econ",
			"winning_probability",
		}
		rows := [][]string{
			{"Smith", "50", "30", "24", "2", "6.5", "0.72"},
		}
		got := parsePlayersFromCSV(header, rows)
		require.Len(t, got, 1)
		require.Equal(t, "Smith", got[0].PlayerName)
		require.Equal(t, 50.0, got[0].RunsScored)
		require.Equal(t, 30.0, got[0].BallsFaced)
		require.Equal(t, 24.0, got[0].Deliveries)
		require.Equal(t, 2.0, got[0].WicketsTaken)
		require.Equal(t, 6.5, got[0].Econ)
		require.Equal(t, 0.72, got[0].WinningProbability)
	})

	t.Run("ignores unknown columns", func(t *testing.T) {
		header := []string{"player_name", "extra_col", "runs_scored"}
		rows := [][]string{{"Kohli", "ignored", "45"}}
		got := parsePlayersFromCSV(header, rows)
		require.Len(t, got, 1)
		require.Equal(t, "Kohli", got[0].PlayerName)
		require.Equal(t, 45.0, got[0].RunsScored)
	})

	t.Run("handles short rows", func(t *testing.T) {
		header := []string{"player_name", "runs_scored", "balls_faced"}
		rows := [][]string{{"Short", "10"}} // missing balls_faced
		got := parsePlayersFromCSV(header, rows)
		require.Len(t, got, 1)
		require.Equal(t, "Short", got[0].PlayerName)
		require.Equal(t, 10.0, got[0].RunsScored)
		require.Equal(t, 0.0, got[0].BallsFaced)
	})

	t.Run("multiple players", func(t *testing.T) {
		header := []string{"player_name", "winning_probability"}
		rows := [][]string{
			{"A", "0.9"},
			{"B", "0.8"},
		}
		got := parsePlayersFromCSV(header, rows)
		require.Len(t, got, 2)
		require.Equal(t, "A", got[0].PlayerName)
		require.Equal(t, 0.9, got[0].WinningProbability)
		require.Equal(t, "B", got[1].PlayerName)
		require.Equal(t, 0.8, got[1].WinningProbability)
	})

	t.Run("all batting/bowling columns", func(t *testing.T) {
		header := []string{
			"player_name", "runs_scored", "balls_faced", "fours_scored", "sixes_scored",
			"batting_position", "strike_rate", "runs_conceded", "deliveries", "wickets_taken", "econ",
		}
		rows := [][]string{{"AllRounder", "40", "25", "4", "2", "3", "160", "30", "24", "1", "7.5"}}
		got := parsePlayersFromCSV(header, rows)
		require.Len(t, got, 1)
		require.Equal(t, "AllRounder", got[0].PlayerName)
		require.Equal(t, 40.0, got[0].RunsScored)
		require.Equal(t, 25.0, got[0].BallsFaced)
		require.Equal(t, 4.0, got[0].FoursScored)
		require.Equal(t, 2.0, got[0].SixesScored)
		require.Equal(t, 3.0, got[0].BattingPosition)
		require.Equal(t, 160.0, got[0].StrikeRate)
		require.Equal(t, 30.0, got[0].RunsConceded)
		require.Equal(t, 24.0, got[0].Deliveries)
		require.Equal(t, 1.0, got[0].WicketsTaken)
		require.Equal(t, 7.5, got[0].Econ)
	})
}

func TestReadAllCSV(t *testing.T) {
	t.Parallel()

	t.Run("reads valid CSV", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "pool.csv")
		require.NoError(t, os.WriteFile(path, []byte("player_name,runs_scored\nSmith,50\nKohli,45"), 0o600))

		recs, err := readAllCSV(path)
		require.NoError(t, err)
		require.Len(t, recs, 3) // header + 2 rows
		require.Equal(t, []string{"player_name", "runs_scored"}, recs[0])
		require.Equal(t, []string{"Smith", "50"}, recs[1])
		require.Equal(t, []string{"Kohli", "45"}, recs[2])
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := readAllCSV("/nonexistent/path.csv")
		require.Error(t, err)
		require.Contains(t, err.Error(), "open pool csv")
	})
}
