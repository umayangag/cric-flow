// Package teamselect: SelectOptimized chooses an XI by maximizing total score over valid combinations.
package teamselect

import (
	"errors"
	"sort"
)

// maxPoolSizeForFullEnum is the maximum pool size for which we enumerate all C(n,11) combinations.
// Above this we use greedy selection + hill-climb swaps to avoid combinatorial blow-up.
const maxPoolSizeForFullEnum = 18

// SelectOptimized chooses a team of c.Size players that maximizes the sum of ScorePlayer(p, w)
// over the team, subject to c.RequireKeeper (at least one keeper) and c.MinBowlers (at least that many bowlers).
// For pools of size <= maxPoolSizeForFullEnum, all valid XIs are enumerated; for larger pools,
// greedy selection is used as a starting point and then hill-climb swaps improve the total score.
// Tie-break: deterministic sort by player names.
func SelectOptimized(pool []Player, w ScoreWeights, c Constraints) ([]Player, error) {
	if c.Size < 1 {
		return nil, errors.New("invalid size")
	}
	if c.MinBowlers < 0 {
		return nil, errors.New("invalid min bowlers")
	}
	if len(pool) < c.Size {
		return nil, errors.New("insufficient pool size")
	}
	keeperCount := countIf(pool, func(p Player) bool { return p.IsKeeper })
	bowlerCount := countIf(pool, func(p Player) bool { return p.IsBowler })
	if c.RequireKeeper && keeperCount == 0 {
		return nil, errors.New("no keeper available")
	}
	if bowlerCount < c.MinBowlers {
		return nil, errors.New("not enough bowlers to satisfy constraint")
	}

	if len(pool) <= maxPoolSizeForFullEnum {
		return selectOptimizedEnum(pool, w, c)
	}
	return selectOptimizedHillClimb(pool, w, c)
}

// selectOptimizedEnum enumerates all C(n, size) combinations, filters to valid XIs, returns the one with max total score.
func selectOptimizedEnum(pool []Player, w ScoreWeights, c Constraints) ([]Player, error) {
	indices := make([]int, 0, c.Size)
	var bestScore float64 = -1
	var bestXI []int

	var gen func(start, remain int)
	gen = func(start, remain int) {
		if remain == 0 {
			// Check constraints
			keepers := 0
			bowlers := 0
			for _, i := range indices {
				if pool[i].IsKeeper {
					keepers++
				}
				if pool[i].IsBowler {
					bowlers++
				}
			}
			if c.RequireKeeper && keepers < 1 {
				return
			}
			if bowlers < c.MinBowlers {
				return
			}
			score := 0.0
			for _, i := range indices {
				score += ScorePlayer(pool[i], w)
			}
			if score > bestScore {
				bestScore = score
				bestXI = make([]int, len(indices))
				copy(bestXI, indices)
			}
			return
		}
		for i := start; i <= len(pool)-remain; i++ {
			indices = append(indices, i)
			gen(i+1, remain-1)
			indices = indices[:len(indices)-1]
		}
	}
	gen(0, c.Size)

	if bestXI == nil {
		return nil, errors.New("no valid XI satisfying constraints")
	}

	out := make([]Player, len(bestXI))
	for j, i := range bestXI {
		out[j] = pool[i]
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// selectOptimizedHillClimb starts with greedy Select then improves by swapping one out, one in.
func selectOptimizedHillClimb(pool []Player, w ScoreWeights, c Constraints) ([]Player, error) {
	team, err := Select(pool, w, c)
	if err != nil {
		return nil, err
	}
	// Build set of indices in team (by name match since we don't have indices)
	inTeam := make(map[string]bool)
	for _, p := range team {
		inTeam[p.Name] = true
	}
	rest := make([]Player, 0, len(pool)-len(team))
	for _, p := range pool {
		if !inTeam[p.Name] {
			rest = append(rest, p)
		}
	}

	totalScore := func(xi []Player) float64 {
		s := 0.0
		for _, p := range xi {
			s += ScorePlayer(p, w)
		}
		return s
	}

	improved := true
	for improved {
		improved = false
		currentScore := totalScore(team)
		for i := 0; i < len(team); i++ {
			for j := 0; j < len(rest); j++ {
				// Try swapping team[i] with rest[j]
				newTeam := make([]Player, len(team))
				copy(newTeam, team)
				newTeam[i] = rest[j]
				newRest := make([]Player, len(rest))
				copy(newRest, rest)
				newRest[j] = team[i]
				if !satisfiesConstraints(newTeam, c) {
					continue
				}
				if totalScore(newTeam) > currentScore {
					team = newTeam
					rest = newRest
					currentScore = totalScore(team)
					improved = true
					break
				}
			}
			if improved {
				break
			}
		}
	}
	sort.Slice(team, func(i, j int) bool { return team[i].Name < team[j].Name })
	return team, nil
}

func satisfiesConstraints(xi []Player, c Constraints) bool {
	keepers := 0
	bowlers := 0
	for _, p := range xi {
		if p.IsKeeper {
			keepers++
		}
		if p.IsBowler {
			bowlers++
		}
	}
	if c.RequireKeeper && keepers < 1 {
		return false
	}
	if bowlers < c.MinBowlers {
		return false
	}
	return true
}
