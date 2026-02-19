package teamselect_test

import (
	"testing"

	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

func mk(name string, bat, bowl float64, isBow, isKeep bool) ts.Player {
	return ts.Player{Name: name, BatScore: bat, BowlScore: bowl, IsBowler: isBow, IsKeeper: isKeep}
}

func totalScore(team []ts.Player, w ts.ScoreWeights) float64 {
	s := 0.0
	for _, p := range team {
		s += ts.ScorePlayer(p, w)
	}
	return s
}

func TestSelectOptimized_Constraints(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := []ts.Player{
		mk("A", 0.9, 0.1, false, false),
		mk("B", 0.7, 0.8, true, false),
		mk("C", 0.6, 0.7, true, false),
		mk("D", 0.5, 0.2, false, false),
		mk("E", 0.4, 0.9, true, false),
		mk("K", 0.3, 0.3, false, true),
		mk("F", 0.35, 0.85, true, false),
		mk("G", 0.2, 0.1, false, false),
		mk("H", 0.25, 0.2, false, false),
		mk("I", 0.15, 0.75, true, false),
		mk("J", 0.1, 0.5, true, false),
	}
	c := ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true}
	if len(pool) < 11 {
		t.Skip("pool too small for size 11")
	}
	// Pad pool to 11+ with duplicates of existing for constraint satisfaction
	pool = append(pool, mk("K2", 0.2, 0.2, false, true))
	team, err := ts.SelectOptimized(pool, w, c)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(team) != 11 {
		t.Fatalf("want size=11 got %d", len(team))
	}
	if countIf(team, func(p ts.Player) bool { return p.IsKeeper }) < 1 {
		t.Fatalf("expected at least one keeper")
	}
	if countIf(team, func(p ts.Player) bool { return p.IsBowler }) < 5 {
		t.Fatalf("expected at least 5 bowlers")
	}
}

func TestSelectOptimized_NoKeeperInPool(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := []ts.Player{
		mk("A", 0.9, 0.1, false, false),
		mk("B", 0.8, 0.8, true, false),
		mk("C", 0.7, 0.7, true, false),
		mk("D", 0.6, 0.2, false, false),
		mk("E", 0.5, 0.9, true, false),
		mk("F", 0.4, 0.5, true, false),
		mk("G", 0.3, 0.3, false, false),
		mk("H", 0.2, 0.4, true, false),
		mk("I", 0.15, 0.6, true, false),
		mk("J", 0.1, 0.1, false, false),
		mk("K", 0.05, 0.2, false, false),
	}
	c := ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true}
	_, err := ts.SelectOptimized(pool, w, c)
	if err == nil {
		t.Fatal("expected error when no keeper in pool")
	}
	if indexOf(err.Error(), "keeper") < 0 {
		t.Fatalf("expected err to mention keeper, got: %v", err)
	}
}

func TestSelectOptimized_NotEnoughBowlers(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := []ts.Player{
		mk("A", 0.9, 0.1, false, false),
		mk("B", 0.8, 0.2, false, false),
		mk("C", 0.7, 0.3, false, true),
		mk("D", 0.6, 0.4, true, false),
		mk("E", 0.5, 0.5, true, false),
		mk("F", 0.4, 0.6, false, false),
		mk("G", 0.3, 0.7, false, false),
		mk("H", 0.2, 0.8, false, false),
		mk("I", 0.15, 0.9, false, false),
		mk("J", 0.1, 0.1, false, false),
		mk("K", 0.05, 0.2, false, false),
	}
	c := ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: false}
	_, err := ts.SelectOptimized(pool, w, c)
	if err == nil {
		t.Fatal("expected error when not enough bowlers")
	}
	if indexOf(err.Error(), "bowler") < 0 {
		t.Fatalf("expected err to mention bowler, got: %v", err)
	}
}

func TestSelectOptimized_ScoreNotWorseThanGreedy(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := make([]ts.Player, 0, 15)
	for i := 0; i < 15; i++ {
		name := string(rune('A'+i))
		bat := 0.9 - float64(i)*0.05
		bowl := 0.1 + float64(i%5)*0.15
		isBowler := i%3 != 0
		isKeeper := i == 10
		pool = append(pool, mk(name, bat, bowl, isBowler, isKeeper))
	}
	c := ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true}
	greedy, errG := ts.Select(pool, w, c)
	optimized, errO := ts.SelectOptimized(pool, w, c)
	if errG != nil {
		t.Fatalf("greedy Select failed: %v", errG)
	}
	if errO != nil {
		t.Fatalf("SelectOptimized failed: %v", errO)
	}
	sG := totalScore(greedy, w)
	sO := totalScore(optimized, w)
	if sO < sG {
		t.Errorf("optimized score %.4f should be >= greedy score %.4f", sO, sG)
	}
}

func TestSelectOptimized_Deterministic(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := []ts.Player{
		mk("A", 0.5, 0.5, true, false),
		mk("B", 0.5, 0.5, true, false),
		mk("C", 0.5, 0.5, true, false),
		mk("D", 0.5, 0.5, true, false),
		mk("E", 0.5, 0.5, true, false),
		mk("F", 0.5, 0.5, false, true),
		mk("G", 0.5, 0.5, false, false),
		mk("H", 0.5, 0.5, false, false),
		mk("I", 0.5, 0.5, false, false),
		mk("J", 0.5, 0.5, false, false),
		mk("K", 0.5, 0.5, false, false),
	}
	c := ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true}
	t1, _ := ts.SelectOptimized(pool, w, c)
	t2, _ := ts.SelectOptimized(pool, w, c)
	if len(t1) != len(t2) {
		t.Fatalf("different lengths %d vs %d", len(t1), len(t2))
	}
	for i := range t1 {
		if t1[i].Name != t2[i].Name {
			t.Errorf("run 1 vs 2 differ at index %d: %s vs %s", i, t1[i].Name, t2[i].Name)
		}
	}
}

func TestSelectOptimized_InsufficientPool(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := []ts.Player{mk("A", 1, 0, false, false)}
	_, err := ts.SelectOptimized(pool, w, ts.Constraints{Size: 11, MinBowlers: 0, RequireKeeper: false})
	if err == nil {
		t.Fatal("expected error for insufficient pool")
	}
	if indexOf(err.Error(), "insufficient") < 0 {
		t.Errorf("expected insufficient pool error, got: %v", err)
	}
}
