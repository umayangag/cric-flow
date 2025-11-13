package teamselect_test

import (
	"context"
	"strings"
	"testing"

	ts "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
)

func TestLoadFromCSV_MoreErrors(t *testing.T) {
	t.Parallel()
	// nil reader
	_, err := ts.LoadFromCSV(nil)
	if err == nil {
		t.Fatalf("expected error for nil reader")
	}
	// missing name
	badName := "name,is_bowler,is_keeper,bat_score,bowl_score\n ,1,0,0.2,0.3\n"
	_, err = ts.LoadFromCSV(strings.NewReader(badName))
	if err == nil || !contains2(err.Error(), "name") {
		t.Fatalf("want invalid name err got %v", err)
	}
	// bad bowl_score
	badBowl := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,0,0,0.2,abc\n"
	_, err = ts.LoadFromCSV(strings.NewReader(badBowl))
	if err == nil || !contains2(err.Error(), "bowl_score") {
		t.Fatalf("want invalid bowl_score err got %v", err)
	}
}

func TestLoadFromDB_ErrPropagate(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{err: context.Canceled}
	_, err := ts.LoadFromDB(context.Background(), repo, 1, "T20", "2019")
	if err == nil {
		t.Fatalf("expected error from repo")
	}
}

func TestSelect_EdgeErrorsAndTies(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	mk := func(name string, bat, bowl float64, isBow, isKeep bool) ts.Player {
		return ts.Player{Name: name, BatScore: bat, BowlScore: bowl, IsBowler: isBow, IsKeeper: isKeep}
	}
	pool := []ts.Player{mk("A", 0.5, 0.5, false, false), mk("B", 0.5, 0.5, false, false)}
	// Equal scores should sort by name ascending deterministically
	team, err := ts.Select(pool, w, ts.Constraints{Size: 1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(team) != 1 || team[0].Name != "A" {
		t.Fatalf("want A selected first, got %#v", team)
	}
	// invalid constraints
	if _, err := ts.Select(pool, w, ts.Constraints{Size: 0}); err == nil {
		t.Fatalf("want error for invalid size")
	}
	if _, err := ts.Select(pool, w, ts.Constraints{Size: 1, MinBowlers: -1}); err == nil {
		t.Fatalf("want error for invalid min bowlers")
	}
	// require keeper but none available
	if _, err := ts.Select(pool, w, ts.Constraints{Size: 1, RequireKeeper: true}); err == nil {
		t.Fatalf("want error for missing keeper")
	}
}

func contains2(s, sub string) bool { return indexOf2(s, sub) >= 0 }
func indexOf2(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
