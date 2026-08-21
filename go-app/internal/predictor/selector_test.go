package predictor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectTop_BasicAndEdgeCases(t *testing.T) {
	// zero team size returns empty
	require.Empty(t, selectTop([]PlayerPrediction{{PlayerName: "A", WinningProbability: 0.9}}, 0))

	// fewer than team size returns all in sorted order
	in1 := []PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "B", WinningProbability: 0.8},
	}
	got1 := selectTop(in1, 5)
	want1 := []PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.9},
		{PlayerName: "B", WinningProbability: 0.8},
	}
	require.Equal(t, want1, got1)

	// deterministic tie-breaker by name
	in2 := []PlayerPrediction{
		{PlayerName: "Zed", WinningProbability: 0.7},
		{PlayerName: "Ann", WinningProbability: 0.7},
	}
	got2 := selectTop(in2, 2)
	require.Equal(t, "Ann", got2[0].PlayerName)
	require.Equal(t, "Zed", got2[1].PlayerName)

	// select top N by probability desc
	in3 := []PlayerPrediction{
		{PlayerName: "A", WinningProbability: 0.1},
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	got3 := selectTop(in3, 2)
	want3 := []PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "C", WinningProbability: 0.5},
	}
	require.Equal(t, want3, got3)

	// does not mutate input slice
	in4 := []PlayerPrediction{
		{PlayerName: "B", WinningProbability: 0.9},
		{PlayerName: "A", WinningProbability: 0.8},
	}
	_ = selectTop(in4, 1)
	require.Equal(t, "B", in4[0].PlayerName)
	require.Equal(t, "A", in4[1].PlayerName)
}
