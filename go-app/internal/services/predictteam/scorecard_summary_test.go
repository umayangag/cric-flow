package predictteam_test

import (
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

func TestComputeScorecardSummary_Team1Wins(t *testing.T) {
	team1 := []predictteam.SelectedPlayer{
		{Runs: 30}, {Runs: 25}, {Runs: 20}, {Runs: 15}, {Runs: 15},
		{Runs: 15}, {Runs: 10}, {Runs: 10}, {Runs: 5}, {Runs: 3}, {Runs: 2},
	}
	team2 := []predictteam.SelectedPlayer{
		{Runs: 28}, {Runs: 22}, {Runs: 18}, {Runs: 14}, {Runs: 14},
		{Runs: 14}, {Runs: 10}, {Runs: 8}, {Runs: 6}, {Runs: 4}, {Runs: 2},
	}
	// team1 batting total = 150, team2 = 140. With extras 5 each: 155 vs 145.
	summary := predictteam.ComputeScorecardSummary(team1, team2, 5, 5, "IND", "AUS")
	if summary.Innings1Total != 155 {
		t.Errorf("innings1_total: want 155 got %.0f", summary.Innings1Total)
	}
	if summary.Innings2Total != 145 {
		t.Errorf("innings2_total: want 145 got %.0f", summary.Innings2Total)
	}
	if summary.PredictedWinner != "IND" {
		t.Errorf("predicted_winner: want IND got %q", summary.PredictedWinner)
	}
	if summary.ExtrasInnings1 != 5 || summary.ExtrasInnings2 != 5 {
		t.Errorf("extras: want 5,5 got %.0f,%.0f", summary.ExtrasInnings1, summary.ExtrasInnings2)
	}
}

func TestComputeScorecardSummary_Team2Wins(t *testing.T) {
	team1 := []predictteam.SelectedPlayer{
		{Runs: 20}, {Runs: 15}, {Runs: 10}, {Runs: 10}, {Runs: 10},
		{Runs: 8}, {Runs: 5}, {Runs: 5}, {Runs: 4}, {Runs: 2}, {Runs: 1},
	}
	team2 := []predictteam.SelectedPlayer{
		{Runs: 25}, {Runs: 22}, {Runs: 18}, {Runs: 15}, {Runs: 12},
		{Runs: 10}, {Runs: 8}, {Runs: 5}, {Runs: 3}, {Runs: 2}, {Runs: 0},
	}
	// team1 total = 90, team2 total = 120. Winner = team2.
	summary := predictteam.ComputeScorecardSummary(team1, team2, 0, 0, "ENG", "PAK")
	if summary.Innings1Total != 90 {
		t.Errorf("innings1_total: want 90 got %.0f", summary.Innings1Total)
	}
	if summary.Innings2Total != 120 {
		t.Errorf("innings2_total: want 120 got %.0f", summary.Innings2Total)
	}
	if summary.PredictedWinner != "PAK" {
		t.Errorf("predicted_winner: want PAK got %q", summary.PredictedWinner)
	}
}

func TestComputeScorecardSummary_Tie(t *testing.T) {
	team1 := []predictteam.SelectedPlayer{
		{Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15},
		{Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15},
	}
	team2 := []predictteam.SelectedPlayer{
		{Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15},
		{Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15}, {Runs: 15},
	}
	// 165 each.
	summary := predictteam.ComputeScorecardSummary(team1, team2, 0, 0, "A", "B")
	if summary.Innings1Total != 165 || summary.Innings2Total != 165 {
		t.Errorf("totals: want 165,165 got %.0f,%.0f", summary.Innings1Total, summary.Innings2Total)
	}
	if summary.PredictedWinner != "" {
		t.Errorf("predicted_winner: want empty on tie got %q", summary.PredictedWinner)
	}
}

func TestComputeScorecardSummary_EmptyTeams(t *testing.T) {
	summary := predictteam.ComputeScorecardSummary(nil, nil, 0, 0, "X", "Y")
	if summary.Innings1Total != 0 || summary.Innings2Total != 0 {
		t.Errorf("empty teams: want 0,0 got %.0f,%.0f", summary.Innings1Total, summary.Innings2Total)
	}
	if summary.PredictedWinner != "" {
		t.Errorf("predicted_winner: want empty got %q", summary.PredictedWinner)
	}
}
