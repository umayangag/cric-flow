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

// hillClimbScoreFunc scores a candidate XI. Returns (score, error).
// score-based selectors return (score, nil); win-probability selectors may return errors.
type hillClimbScoreFunc func(candidate []Player) (float64, error)

// hillClimbSwap performs iterative single-swap hill climbing on team vs rest,
// using scoreFunc to evaluate candidates and respecting constraints.
// maxIter caps total passes (0 = unlimited, stop when no improvement).
// maxEvals caps total scoreFunc calls across all iterations (0 = unlimited).
// This budget prevents excessive external calls when scoreFunc is an HTTP round-trip.
func hillClimbSwap(team, rest []Player, c Constraints, scoreFunc hillClimbScoreFunc, maxIter, maxEvals int) []Player {
	currentScore, err := scoreFunc(team)
	if err != nil {
		return team
	}
	evalCount := 1

	for iter := 0; maxIter == 0 || iter < maxIter; iter++ {
		improved := false
		budgetExhausted := false
		for i := range team {
			for j := range rest {
				if maxEvals > 0 && evalCount >= maxEvals {
					budgetExhausted = true
					break
				}
				newTeam := make([]Player, len(team))
				copy(newTeam, team)
				newTeam[i] = rest[j]
				if !satisfiesConstraints(newTeam, c) {
					continue
				}
				s, err := scoreFunc(newTeam)
				evalCount++
				if err != nil {
					continue
				}
				if s > currentScore {
					newRest := make([]Player, len(rest))
					copy(newRest, rest)
					newRest[j] = team[i]
					team = newTeam
					rest = newRest
					currentScore = s
					improved = true
					break
				}
			}
			if improved || budgetExhausted {
				break
			}
		}
		if !improved || budgetExhausted {
			break
		}
	}
	return team
}

// splitTeamAndRest partitions pool into selected team and remaining players.
func splitTeamAndRest(pool, team []Player) []Player {
	inTeam := make(map[string]bool, len(team))
	for _, p := range team {
		inTeam[p.Name] = true
	}
	rest := make([]Player, 0, len(pool)-len(team))
	for _, p := range pool {
		if !inTeam[p.Name] {
			rest = append(rest, p)
		}
	}
	return rest
}

// selectOptimizedHillClimb starts with greedy Select then improves by swapping one out, one in.
func selectOptimizedHillClimb(pool []Player, w ScoreWeights, c Constraints) ([]Player, error) {
	team, err := Select(pool, w, c)
	if err != nil {
		return nil, err
	}
	rest := splitTeamAndRest(pool, team)

	scoreFunc := func(xi []Player) (float64, error) {
		s := 0.0
		for _, p := range xi {
			s += ScorePlayer(p, w)
		}
		return s, nil
	}
	team = hillClimbSwap(team, rest, c, scoreFunc, 0, 0)
	sort.Slice(team, func(i, j int) bool { return team[i].Name < team[j].Name })
	return team, nil
}

// WinProbEvalFunc evaluates the win probability for a candidate XI.
// Returns team1 win probability in [0,1] or an error.
type WinProbEvalFunc func(candidateNames []string) (float64, error)

// winProbSwapIterations returns the configured hill-climb iteration cap for win-probability selection.
func winProbSwapIterations() int {
	return config.SelectionMaxWinProbSwapIterations(config.Load())
}

// winProbEvalBudget returns the configured ML evaluation call budget for win-probability selection.
func winProbEvalBudget() int {
	return config.SelectionMaxWinProbEvalBudget(config.Load())
}

// SelectByWinProbability selects a team that maximizes win probability using
// hill-climb optimization. Starts from a greedy seed (ScorePlayer-based),
// then iteratively swaps players to improve the win probability.
// Both the iteration count and total ML evaluation calls are capped via config
// to prevent excessive load on the prediction service.
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
	rest := splitTeamAndRest(pool, team)

	scoreFunc := func(xi []Player) (float64, error) {
		names := make([]string, len(xi))
		for i, p := range xi {
			names[i] = p.Name
		}
		return evalFunc(names)
	}

	team = hillClimbSwap(team, rest, c, scoreFunc, winProbSwapIterations(), winProbEvalBudget())
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
