package predictor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePlayersCSV_Basic(t *testing.T) {
	csv := "player_name,runs_scored,balls_faced,fours_scored,sixes_scored,batting_position,strike_rate,runs_conceded,deliveries,wickets_taken,econ,winning_probability\n" +
		"Alice,30,25,3,1,3,120,20,24,1,5.0,0.65\n" +
		"Bob,10,12,1,0,5,83.3,35,18,2,6.5,0.55\n"

	players, err := parsePlayersCSV(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, players, 2)
	require.Equal(t, "Alice", players[0].PlayerName)
	require.Equal(t, "Bob", players[1].PlayerName)
	require.Equal(t, float64(30), players[0].RunsScored)
	require.Equal(t, float64(2), players[1].WicketsTaken)
}

func TestParsePlayersCSV_Empty(t *testing.T) {
	_, err := parsePlayersCSV(strings.NewReader(""))
	require.Error(t, err)
}
