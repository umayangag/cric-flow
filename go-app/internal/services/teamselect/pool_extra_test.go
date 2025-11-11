package teamselect_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	ts "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
)

// fakeRepo implements db.TeamSelectRepo for tests
type fakeRepo2 struct {
	players []db.PoolPlayer
	err     error
}

func (f *fakeRepo2) LoadPool(context.Context, int64, string, string) ([]db.PoolPlayer, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.players, nil
}

func TestLoadFromCSV_EmptyAndHeaderErrors(t *testing.T) {
	t.Parallel()
	// empty
	if _, err := ts.LoadFromCSV(strings.NewReader("")); err == nil {
		t.Fatalf("want error for empty csv")
	}
	// unexpected header
	badHeader := "a,b,c\n1,2,3\n"
	if _, err := ts.LoadFromCSV(strings.NewReader(badHeader)); err == nil {
		t.Fatalf("want error for unexpected header")
	}
	// too few columns
	fewCols := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,1\n"
	if _, err := ts.LoadFromCSV(strings.NewReader(fewCols)); err == nil {
		t.Fatalf("want error for too few columns")
	}
	// bad bat_score
	badBat := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,0,0,abc,0.1\n"
	if _, err := ts.LoadFromCSV(strings.NewReader(badBat)); err == nil {
		t.Fatalf("want error for bad bat_score")
	}
}

func TestLoadFromDB_InvalidArgsAndSuccess(t *testing.T) {
	t.Parallel()
	// nil repo
	if _, err := ts.LoadFromDB(context.Background(), nil, 1, "T20", "2019"); err == nil {
		t.Fatalf("want error for nil repo")
	}
	// invalid args
	repo := &fakeRepo2{}
	if _, err := ts.LoadFromDB(context.Background(), repo, 0, "T20", "2019"); err == nil {
		t.Fatalf("want error for invalid match id")
	}
	if _, err := ts.LoadFromDB(context.Background(), repo, 1, "", "2019"); err == nil {
		t.Fatalf("want error for empty format")
	}
	if _, err := ts.LoadFromDB(context.Background(), repo, 1, "T20", ""); err == nil {
		t.Fatalf("want error for empty season")
	}
	// success maps DTOs to Player
	repo.players = []db.PoolPlayer{{Name: "A", IsBowler: true, BatScore: 0.3, BowlScore: 0.7}, {Name: "K", IsKeeper: true, BatScore: 0.5, BowlScore: 0.2}}
	ps, err := ts.LoadFromDB(context.Background(), repo, 1, "T20", "2019")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ps) != 2 || ps[0].Name != "A" || !ps[0].IsBowler || !ps[1].IsKeeper {
		t.Fatalf("unexpected players: %#v", ps)
	}
}

func TestSelect_NoKeeperAvailable(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := []ts.Player{{Name: "A", BatScore: 0.9}, {Name: "B", BatScore: 0.8}}
	if _, err := ts.Select(pool, w, ts.Constraints{Size: 1, RequireKeeper: true}); err == nil {
		t.Fatalf("want error when keeper required but none available")
	}
}

func TestSelect_BowlerReplacementFallback(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	// team of size 2, need 1 bowler; only one candidate bowler in rest should replace a non-bowler
	pool := []ts.Player{{Name: "A", BatScore: 0.9}, {Name: "B", BatScore: 0.8}, {Name: "C", BowlScore: 0.9, IsBowler: true}}
	team, err := ts.Select(pool, w, ts.Constraints{Size: 2, MinBowlers: 1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if countBowl(team) < 1 {
		t.Fatalf("want at least 1 bowler, got %#v", team)
	}
}

func countBowl(ps []ts.Player) int {
	n := 0
	for _, p := range ps {
		if p.IsBowler {
			n++
		}
	}
	return n
}

func TestLoadFromDB_ErrorPropagation(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo2{err: errors.New("boom")}
	if _, err := ts.LoadFromDB(context.Background(), repo, 1, "ODI", "2019"); err == nil {
		t.Fatalf("want error propagated from repo")
	}
}
