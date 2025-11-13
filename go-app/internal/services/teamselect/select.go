package teamselect

import (
	"errors"
	"sort"
)

// Constraints captures requirements for selection.
type Constraints struct {
	Size          int
	MinBowlers    int
	RequireKeeper bool
}

// Select chooses a team from the pool using a deterministic scoring and
// constraint satisfaction approach. It returns an error when constraints
// cannot be satisfied.
func Select(pool []Player, w ScoreWeights, c Constraints) ([]Player, error) {
	if c.Size < 1 {
		return nil, errors.New("invalid size")
	}
	if c.MinBowlers < 0 {
		return nil, errors.New("invalid min bowlers")
	}
	if len(pool) < c.Size {
		return nil, errors.New("insufficient pool size")
	}
	// Sort by score desc, then by name asc for deterministic tie-breaking.
	scored := make([]Player, len(pool))
	copy(scored, pool)
	sort.Slice(scored, func(i, j int) bool {
		si := ScorePlayer(scored[i], w)
		sj := ScorePlayer(scored[j], w)
		if si == sj {
			return scored[i].Name < scored[j].Name
		}
		return si > sj
	})
	// First pass: greedily take best players respecting constraints minimally.
	team := make([]Player, 0, c.Size)
	bowCount := 0
	keeperSeen := false
	for _, p := range scored {
		if len(team) == c.Size {
			break
		}
		team = append(team, p)
		if p.IsBowler {
			bowCount++
		}
		if p.IsKeeper {
			keeperSeen = true
		}
	}
	// If constraints unmet, try to swap in candidates from the remainder.
	// Ensure we attempt to satisfy keeper first (if required), then bowlers.
	rest := scored[len(team):]
	if c.RequireKeeper && !keeperSeen {
		idx := indexFirst(rest, func(p Player) bool { return p.IsKeeper })
		if idx < 0 {
			return nil, errors.New("no keeper available")
		}
		// Replace the lowest-scored non-keeper in team with this keeper.
		rep := indexLast(team, func(p Player) bool { return !p.IsKeeper })
		if rep < 0 {
			return nil, errors.New("cannot satisfy keeper constraint")
		}
		team[rep] = rest[idx]
		rest = append(rest[:idx], rest[idx+1:]...)
		// recompute bowlCount in case swap affected it
		bowCount = countIf(team, func(p Player) bool { return p.IsBowler })
	}
	if bowCount < c.MinBowlers {
		need := c.MinBowlers - bowCount
		for need > 0 {
			idx := indexFirst(rest, func(p Player) bool { return p.IsBowler })
			if idx < 0 {
				return nil, errors.New("not enough bowlers to satisfy constraint")
			}
			// Replace the lowest-scored non-bowler (prefer non-keeper to keep keeper if required)
			rep := indexLast(team, func(p Player) bool { return !p.IsBowler && (!c.RequireKeeper || !p.IsKeeper) })
			if rep < 0 {
				// fallback: replace absolute lowest that isn't a bowler (even if keeper not required)
				rep = indexLast(team, func(p Player) bool { return !p.IsBowler })
				if rep < 0 {
					return nil, errors.New("cannot replace to satisfy bowler constraint")
				}
			}
			team[rep] = rest[idx]
			rest = append(rest[:idx], rest[idx+1:]...)
			need--
		}
	}
	// Final sanity check
	if c.RequireKeeper && countIf(team, func(p Player) bool { return p.IsKeeper }) == 0 {
		return nil, errors.New("keeper constraint not met")
	}
	if countIf(team, func(p Player) bool { return p.IsBowler }) < c.MinBowlers {
		return nil, errors.New("min bowlers constraint not met")
	}
	// keep deterministic order: sort by name for output stability
	sort.Slice(team, func(i, j int) bool { return team[i].Name < team[j].Name })
	return team, nil
}

func indexFirst(ps []Player, pred func(Player) bool) int {
	for i, p := range ps {
		if pred(p) {
			return i
		}
	}
	return -1
}

func indexLast(ps []Player, pred func(Player) bool) int {
	for i := len(ps) - 1; i >= 0; i-- {
		if pred(ps[i]) {
			return i
		}
	}
	return -1
}

func countIf(ps []Player, pred func(Player) bool) int {
	n := 0
	for _, p := range ps {
		if pred(p) {
			n++
		}
	}
	return n
}
