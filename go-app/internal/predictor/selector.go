package predictor

import (
	"sort"
)

// selectTop returns the top N players by WinningProbability.
// It is pure and deterministic. For ties, it uses PlayerName ascending as a tie-breaker
// to keep unit tests stable across runs and environments.
func selectTop(players []PlayerPrediction, teamSize int) []PlayerPrediction {
	if teamSize <= 0 || len(players) == 0 {
		return []PlayerPrediction{}
	}

	// Work on a copy to avoid mutating the input slice.
	cp := append([]PlayerPrediction(nil), players...)
	sort.Slice(cp, func(i, j int) bool {
		li, lj := cp[i], cp[j]
		if li.WinningProbability == lj.WinningProbability {
			return li.PlayerName < lj.PlayerName
		}
		return li.WinningProbability > lj.WinningProbability
	})
	if teamSize > len(cp) {
		teamSize = len(cp)
	}
	return cp[:teamSize]
}
