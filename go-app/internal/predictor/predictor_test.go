package predictor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// TestCalculateOverallPerformanceWithConfig verifies team aggregates are computed
// correctly from player predictions using the provided config. We avoid any IO
// by constructing the config inline.
func TestCalculateOverallPerformanceWithConfig(t *testing.T) {
	testCases := []struct {
		name     string
		players  []PlayerPrediction
		matchID  int64
		teamSize int
		extras   float64
		expect   Team
	}{
		{
			name: "two players typical",
			players: []PlayerPrediction{
				{RunsScored: 30, BallsFaced: 20, RunsConceded: 10, Deliveries: 12, WicketsTaken: 1},
				{RunsScored: 20, BallsFaced: 15, RunsConceded: 25, Deliveries: 24, WicketsTaken: 2},
			},
			matchID:  123,
			teamSize: 11,
			extras:   5.0,
			expect: Team{
				TotalWickets: 10, // fixed for now in implementation
				Extras:       5.0,
				MatchNumber:  123,
			},
		},
		{
			name: "single player",
			players: []PlayerPrediction{
				{RunsScored: 50, BallsFaced: 35, RunsConceded: 40, Deliveries: 24, WicketsTaken: 0},
			},
			matchID:  7,
			teamSize: 11,
			extras:   4.0,
			expect: Team{
				TotalWickets: 10,
				Extras:       4.0,
				MatchNumber:  7,
			},
		},
		{
			name:    "empty players returns extras only",
			players: []PlayerPrediction{},
			matchID: 42,
			// teamSize doesn't matter here; no division performed
			teamSize: 11,
			extras:   3.0,
			expect: Team{
				TotalWickets: 10,
				Extras:       3.0,
				MatchNumber:  42,
				TotalScore:   3.0,
				Target:       0,
				TotalBalls:   0,
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Predictor.TeamSize = tc.teamSize

			team := CalculateOverallPerformanceWithConfig(cfg, tc.players, tc.matchID, tc.extras)

			// Compute expected totals inline for assertion.
			if len(tc.players) == 0 {
				require.Equal(t, tc.expect.TotalScore, team.TotalScore)
				require.Equal(t, tc.expect.Target, team.Target)
				require.Equal(t, tc.expect.TotalBalls, team.TotalBalls)
				require.Equal(t, tc.expect.TotalWickets, team.TotalWickets)
				require.Equal(t, tc.expect.Extras, team.Extras)
				require.Equal(t, tc.expect.MatchNumber, team.MatchNumber)
				require.Empty(t, team.Players)
				return
			}

			var totalRunsScored, totalBallsFaced, totalRunsConceded float64
			for _, p := range tc.players {
				totalRunsScored += p.RunsScored
				totalBallsFaced += p.BallsFaced
				totalRunsConceded += p.RunsConceded
			}
			magic := float64(tc.teamSize) / float64(len(tc.players))
			expectedTotalScore := totalRunsScored*magic + tc.extras
			expectedTarget := totalRunsConceded * magic
			expectedTotalBalls := totalBallsFaced * magic

			require.Equal(t, expectedTotalScore, team.TotalScore)
			require.Equal(t, expectedTarget, team.Target)
			require.Equal(t, expectedTotalBalls, team.TotalBalls)
			require.Equal(t, tc.expect.TotalWickets, team.TotalWickets)
			require.Equal(t, tc.expect.Extras, team.Extras)
			require.Equal(t, tc.expect.MatchNumber, team.MatchNumber)
			require.Len(t, team.Players, len(tc.players))
		})
	}
}
