// Package teamselect: SelectOptimized chooses an XI by maximizing total score over valid combinations.
package teamselect

import (
	"errors"
	"sort"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// maxPoolSizeForFullEnum returns the maximum pool size for full enumeration (from config or default).
// Above this we use greedy selection + hill-climb swaps to avoid combinatorial blow-up.
func maxPoolSizeForFullEnum() int {
	return config.SelectionMaxPoolSizeForFullEnum(config.Load())
}

// SelectOptimized chooses a team of c.Size players that maximizes the sum of ScorePlayer(p, w)
// over the team, subject to c.RequireKeeper (at least one keeper) and c.MinBowlers (at least that many bowlers).
// For pools of size <= maxPoolSizeForFullEnum(), all valid XIs are enumerated; for larger pools,
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

	if len(pool) <= maxPoolSizeForFullEnum() {
		return selectOptimizedEnum(pool, w, c)
	}
	return selectOptimizedHillClimb(pool, w, c)
}

// SelectTopK returns up to k valid XIs sorted by total score descending.
// When pool size <= maxPoolSizeForFullEnum, all valid XIs are enumerated and the top k returned.
// When pool is larger, only the single best XI (from hill-climb) is returned, so k is effectively 1.
// k must be >= 1.
func SelectTopK(pool []Player, w ScoreWeights, c Constraints, k int) ([][]Player, error) {
	if k < 1 {
		return nil, errors.New("k must be at least 1")
	}
	if c.Size < 1 || c.MinBowlers < 0 || len(pool) < c.Size {
		return nil, errors.New("invalid constraints or insufficient pool")
	}
	keeperCount := countIf(pool, func(p Player) bool { return p.IsKeeper })
	bowlerCount := countIf(pool, func(p Player) bool { return p.IsBowler })
	if c.RequireKeeper && keeperCount == 0 {
		return nil, errors.New("no keeper available")
	}
	if bowlerCount < c.MinBowlers {
		return nil, errors.New("not enough bowlers to satisfy constraint")
	}

	if len(pool) <= maxPoolSizeForFullEnum() {
		all := enumerateValidXIs(pool, w, c)
		if len(all) == 0 {
			return nil, errors.New("no valid XI satisfying constraints")
		}
		// all is already sorted by score desc; take first k
		n := k
		if n > len(all) {
			n = len(all)
		}
		out := make([][]Player, n)
		for i := 0; i < n; i++ {
			xi := make([]Player, len(all[i]))
			copy(xi, all[i])
			sort.Slice(xi, func(a, b int) bool { return xi[a].Name < xi[b].Name })
			out[i] = xi
		}
		return out, nil
	}
	// Large pool: return only the best XI
	best, err := selectOptimizedHillClimb(pool, w, c)
	if err != nil {
		return nil, err
	}
	return [][]Player{best}, nil
}

// enumeratedXI holds a valid XI and its total score for sorting.
type enumeratedXI struct {
	score float64
	xi    []int
}

// enumerateValidXIs enumerates all valid XIs for pool size <= maxPoolSizeForFullEnum,
// sorted by total score descending.
func enumerateValidXIs(pool []Player, w ScoreWeights, c Constraints) [][]Player {
	indices := make([]int, 0, c.Size)
	var list []enumeratedXI

	var gen func(start, remain int)
	gen = func(start, remain int) {
		if remain == 0 {
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
			xi := make([]int, len(indices))
			copy(xi, indices)
			list = append(list, enumeratedXI{score: score, xi: xi})
			return
		}
		for i := start; i <= len(pool)-remain; i++ {
			indices = append(indices, i)
			gen(i+1, remain-1)
			indices = indices[:len(indices)-1]
		}
	}
	gen(0, c.Size)

	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		// Tie-break: compare names of first differing player
		for t := 0; t < c.Size; t++ {
			ni, nj := pool[list[i].xi[t]].Name, pool[list[j].xi[t]].Name
			if ni != nj {
				return ni < nj
			}
		}
		return false
	})

	out := make([][]Player, len(list))
	for i, e := range list {
		team := make([]Player, len(e.xi))
		for j, idx := range e.xi {
			team[j] = pool[idx]
		}
		out[i] = team
	}
	return out
}

// selectOptimizedEnum enumerates all C(n, size) combinations, filters to valid XIs, returns the one with max total score.
func selectOptimizedEnum(pool []Player, w ScoreWeights, c Constraints) ([]Player, error) {
	all := enumerateValidXIs(pool, w, c)
	if len(all) == 0 {
		return nil, errors.New("no valid XI satisfying constraints")
	}
	out := all[0]
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

// WinProbEvalFunc evaluates the win probability for a candidate XI.
// Returns team1 win probability in [0,1] or an error.
type WinProbEvalFunc func(candidateNames []string) (float64, error)

// SelectByWinProbability selects a team that maximizes win probability using
// hill-climb optimization. Starts from a greedy seed (ScorePlayer-based),
// then iteratively swaps players to improve the win probability.
func SelectByWinProbability(pool []Player, w ScoreWeights, c Constraints, evalFunc WinProbEvalFunc) ([]Player, error) {
	if c.Size < 1 {
		return nil, errors.New("invalid size")
	}
	if len(pool) < c.Size {
		return nil, errors.New("insufficient pool size")
	}

	team, err := Select(pool, w, c)
	if err != nil {
		return nil, err
	}

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

	teamNames := func(xi []Player) []string {
		names := make([]string, len(xi))
		for i, p := range xi {
			names[i] = p.Name
		}
		return names
	}

	currentWinProb, err := evalFunc(teamNames(team))
	if err != nil {
		return team, nil
	}

	const maxIterations = 50
	for iter := 0; iter < maxIterations; iter++ {
		improved := false
		for i := 0; i < len(team); i++ {
			for j := 0; j < len(rest); j++ {
				newTeam := make([]Player, len(team))
				copy(newTeam, team)
				newTeam[i] = rest[j]
				if !satisfiesConstraints(newTeam, c) {
					continue
				}
				p, err := evalFunc(teamNames(newTeam))
				if err != nil {
					continue
				}
				if p > currentWinProb {
					newRest := make([]Player, len(rest))
					copy(newRest, rest)
					newRest[j] = team[i]
					team = newTeam
					rest = newRest
					currentWinProb = p
					improved = true
					break
				}
			}
			if improved {
				break
			}
		}
		if !improved {
			break
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
