package teamselect_test

import (
	"strings"
	"testing"

	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// mkPool returns a pool of 11 players: 6 batters, 5 bowlers, 1 keeper.
func mkPool() []ts.Player {
	return []ts.Player{
		{Name: "A", BatScore: 0.9, IsBowler: false, IsKeeper: true},
		{Name: "B", BatScore: 0.85, IsBowler: false},
		{Name: "C", BatScore: 0.8, IsBowler: false},
		{Name: "D", BatScore: 0.75, IsBowler: false},
		{Name: "E", BatScore: 0.7, IsBowler: false},
		{Name: "F", BatScore: 0.65, IsBowler: false},
		{Name: "G", BowlScore: 0.9, IsBowler: true},
		{Name: "H", BowlScore: 0.8, IsBowler: true},
		{Name: "I", BowlScore: 0.7, IsBowler: true},
		{Name: "J", BowlScore: 0.6, IsBowler: true},
		{Name: "K", BowlScore: 0.5, IsBowler: true},
	}
}

func TestSelectOptimized_ValidPool(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()
	sel, err := ts.SelectOptimized(pool, w, ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true})
	if err != nil {
		t.Fatalf("SelectOptimized: %v", err)
	}
	if len(sel) != 11 {
		t.Fatalf("want 11 selected, got %d", len(sel))
	}
	bowlers := 0
	keepers := 0
	for _, p := range sel {
		if p.IsBowler {
			bowlers++
		}
		if p.IsKeeper {
			keepers++
		}
	}
	if bowlers < 5 {
		t.Fatalf("want at least 5 bowlers, got %d", bowlers)
	}
	if keepers < 1 {
		t.Fatalf("want at least 1 keeper, got %d", keepers)
	}
}

func TestSelectOptimized_Errors(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()

	tests := []struct {
		name   string
		pool   []ts.Player
		c      ts.Constraints
		errStr string
	}{
		{"invalid size", pool, ts.Constraints{Size: 0, MinBowlers: 5}, "invalid size"},
		{"insufficient pool", pool[:5], ts.Constraints{Size: 11, MinBowlers: 2}, "insufficient pool"},
		{"no keeper", func() []ts.Player {
			// Pool of 11 with no keeper
			p := mkPool()
			p[0].IsKeeper = false
			return p
		}(), ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true}, "no keeper"},
		{"not enough bowlers", pool[:8], ts.Constraints{Size: 8, MinBowlers: 6}, "not enough bowlers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ts.SelectOptimized(tt.pool, w, tt.c)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.errStr)
			}
			if !strings.Contains(err.Error(), tt.errStr) {
				t.Fatalf("err %q does not contain %q", err.Error(), tt.errStr)
			}
		})
	}
}

func TestSelectTopK_Valid(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()
	xis, err := ts.SelectTopK(pool, w, ts.Constraints{Size: 11, MinBowlers: 5, RequireKeeper: true}, 3)
	if err != nil {
		t.Fatalf("SelectTopK: %v", err)
	}
	if len(xis) < 1 || len(xis) > 3 {
		t.Fatalf("want 1-3 XIs, got %d", len(xis))
	}
	for _, xi := range xis {
		if len(xi) != 11 {
			t.Fatalf("each XI should have 11 players, got %d", len(xi))
		}
	}
}

func TestSelectTopK_InvalidK(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := mkPool()
	_, err := ts.SelectTopK(pool, w, ts.Constraints{Size: 11, MinBowlers: 5}, 0)
	if err == nil {
		t.Fatalf("expected error for k=0")
	}
}

