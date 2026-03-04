package predictteam

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRescaleTeamPredictionsToWinProbability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		team1     []SelectedPlayer
		team2     []SelectedPlayer
		extras1   float64
		extras2   float64
		winProb   float64
		wantRuns1 float64 // sum of team1 runs after rescale
		wantRuns2 float64 // sum of team2 runs after rescale
	}{
		{
			name: "equal_probability_no_change",
			team1: []SelectedPlayer{
				{Runs: 30}, {Runs: 20},
			},
			team2: []SelectedPlayer{
				{Runs: 30}, {Runs: 20},
			},
			extras1:   0,
			extras2:   0,
			winProb:   0.5,
			wantRuns1: 50,
			wantRuns2: 50,
		},
		{
			name: "team1_favored_70_percent",
			team1: []SelectedPlayer{
				{Runs: 50},
			},
			team2: []SelectedPlayer{
				{Runs: 50},
			},
			extras1:   0,
			extras2:   0,
			winProb:   0.7,
			wantRuns1: 70,
			wantRuns2: 30,
		},
		{
			name: "with_extras",
			team1: []SelectedPlayer{
				{Runs: 40},
			},
			team2: []SelectedPlayer{
				{Runs: 40},
			},
			extras1:   10,
			extras2:   10,
			winProb:   0.5,
			wantRuns1: 40, // total=100, target1=50-10=40
			wantRuns2: 40, // target2=50-10=40
		},
		{
			name:      "zero_total_innings_no_panic",
			team1:     []SelectedPlayer{{Runs: 0}},
			team2:     []SelectedPlayer{{Runs: 0}},
			extras1:   0,
			extras2:   0,
			winProb:   0.6,
			wantRuns1: 0,
			wantRuns2: 0,
		},
		{
			name: "team2_favored",
			team1: []SelectedPlayer{
				{Runs: 25}, {Runs: 25},
			},
			team2: []SelectedPlayer{
				{Runs: 25}, {Runs: 25},
			},
			extras1:   0,
			extras2:   0,
			winProb:   0.3,
			wantRuns1: 30,
			wantRuns2: 70,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange: deep copy slices so parallel tests don't interfere.
			t1 := make([]SelectedPlayer, len(tt.team1))
			copy(t1, tt.team1)
			t2 := make([]SelectedPlayer, len(tt.team2))
			copy(t2, tt.team2)

			// Act
			rescaleTeamPredictionsToWinProbability(t1, t2, tt.extras1, tt.extras2, tt.winProb)

			// Assert
			var sumRuns1, sumRuns2 float64
			for _, p := range t1 {
				sumRuns1 += p.Runs
			}
			for _, p := range t2 {
				sumRuns2 += p.Runs
			}
			assert.InDelta(t, tt.wantRuns1, sumRuns1, 0.01, "team1 runs mismatch")
			assert.InDelta(t, tt.wantRuns2, sumRuns2, 0.01, "team2 runs mismatch")
		})
	}
}
