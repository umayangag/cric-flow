package teamselect_test

import (
	"strings"
	"testing"

	ts "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
)

func TestSelect_CannotReplaceToSatisfyBowlers(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	// Top two are bowlers, rest also has bowlers but team has no non-bowler to replace
	pool := []ts.Player{
		{Name: "B1", BowlScore: 0.9, IsBowler: true},
		{Name: "B2", BowlScore: 0.8, IsBowler: true},
		{Name: "B3", BowlScore: 0.7, IsBowler: true},
	}
	if _, err := ts.Select(pool, w, ts.Constraints{Size: 2, MinBowlers: 3}); err == nil {
		t.Fatalf("want error when cannot replace to satisfy bowler constraint")
	}
}

func TestLoadFromCSV_EmptyFloatsAndBoolVariants(t *testing.T) {
	t.Parallel()
	csv := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,yes,no,,\nB,true,1, ,0.4\nC,0,YES,0.3, \n"
	ps, err := ts.LoadFromCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ps) != 3 {
		t.Fatalf("want 3 players got %d", len(ps))
	}
	// First row empty floats become 0
	if ps[0].BatScore != 0 || ps[0].BowlScore != 0 || !ps[0].IsBowler || ps[0].IsKeeper {
		t.Fatalf("unexpected row A: %#v", ps[0])
	}
	// Second row space-only bat score becomes 0 after trim; is_keeper=1 recognized
	if ps[1].BatScore != 0 || ps[1].BowlScore != 0.4 || !ps[1].IsBowler || !ps[1].IsKeeper {
		t.Fatalf("unexpected row B: %#v", ps[1])
	}
	// Third row keeper YES recognized
	if !ps[2].IsKeeper {
		t.Fatalf("expected C to be keeper")
	}
}
