package predictor

import (
	"strings"
	"testing"
)

func TestParsePlayersCSV_Basic(t *testing.T) {
	csv := "player_name,runs_scored,balls_faced,fours_scored,sixes_scored,batting_position,strike_rate,runs_conceded,deliveries,wickets_taken,econ,winning_probability\n" +
		"Alice,30,25,3,1,3,120,20,24,1,5.0,0.65\n" +
		"Bob,10,12,1,0,5,83.3,35,18,2,6.5,0.55\n"

	players, err := parsePlayersCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(players) != 2 {
		t.Fatalf("want 2 players, got %d", len(players))
	}
	if players[0].PlayerName != "Alice" || players[1].PlayerName != "Bob" {
		t.Fatalf("unexpected names: %#v", players)
	}
	if players[0].RunsScored != 30 || players[1].WicketsTaken != 2 {
		t.Fatalf("unexpected parsed values: %#v", players)
	}
}

func TestParsePlayersCSV_Empty(t *testing.T) {
	_, err := parsePlayersCSV(strings.NewReader(""))
	if err == nil {
		t.Fatalf("expected error for empty csv")
	}
}
