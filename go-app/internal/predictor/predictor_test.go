package predictor

import (
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// TestCalculateOverallPerformanceWithConfig verifies team aggregates are computed
// correctly from player predictions using the provided config. We avoid any IO
// by constructing the config inline.
func TestCalculateOverallPerformanceWithConfig(t *testing.T) {
	tests := []struct {
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Predictor.TeamSize = tc.teamSize

			team := CalculateOverallPerformanceWithConfig(cfg, tc.players, tc.matchID, tc.extras)

			// Compute expected totals inline for assertion.
			if len(tc.players) == 0 {
				if team.TotalScore != tc.expect.TotalScore {
					t.Fatalf("TotalScore mismatch: got %.2f want %.2f", team.TotalScore, tc.expect.TotalScore)
				}
				if team.Target != tc.expect.Target {
					t.Fatalf("Target mismatch: got %.2f want %.2f", team.Target, tc.expect.Target)
				}
				if team.TotalBalls != tc.expect.TotalBalls {
					t.Fatalf("TotalBalls mismatch: got %.2f want %.2f", team.TotalBalls, tc.expect.TotalBalls)
				}
				if team.TotalWickets != tc.expect.TotalWickets {
					t.Fatalf("TotalWickets mismatch: got %.0f want %.0f", team.TotalWickets, tc.expect.TotalWickets)
				}
				if team.Extras != tc.expect.Extras {
					t.Fatalf("Extras mismatch: got %.2f want %.2f", team.Extras, tc.expect.Extras)
				}
				if team.MatchNumber != tc.expect.MatchNumber {
					t.Fatalf("MatchNumber mismatch: got %d want %d", team.MatchNumber, tc.expect.MatchNumber)
				}
				if len(team.Players) != 0 {
					t.Fatalf("Players length mismatch: got %d want %d", len(team.Players), 0)
				}
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

			if team.TotalScore != expectedTotalScore {
				t.Fatalf("TotalScore mismatch: got %.2f want %.2f", team.TotalScore, expectedTotalScore)
			}
			if team.Target != expectedTarget {
				t.Fatalf("Target mismatch: got %.2f want %.2f", team.Target, expectedTarget)
			}
			if team.TotalBalls != expectedTotalBalls {
				t.Fatalf("TotalBalls mismatch: got %.2f want %.2f", team.TotalBalls, expectedTotalBalls)
			}
			if team.TotalWickets != tc.expect.TotalWickets {
				t.Fatalf("TotalWickets mismatch: got %.0f want %.0f", team.TotalWickets, tc.expect.TotalWickets)
			}
			if team.Extras != tc.expect.Extras {
				t.Fatalf("Extras mismatch: got %.2f want %.2f", team.Extras, tc.expect.Extras)
			}
			if team.MatchNumber != tc.expect.MatchNumber {
				t.Fatalf("MatchNumber mismatch: got %d want %d", team.MatchNumber, tc.expect.MatchNumber)
			}
			if len(team.Players) != len(tc.players) {
				t.Fatalf("Players length mismatch: got %d want %d", len(team.Players), len(tc.players))
			}
		})
	}
}
