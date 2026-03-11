package teamselect_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
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
	_, err := ts.LoadFromCSV(strings.NewReader(""))
	require.Error(t, err, "want error for empty csv")
	badHeader := "a,b,c\n1,2,3\n"
	_, err = ts.LoadFromCSV(strings.NewReader(badHeader))
	require.Error(t, err, "want error for unexpected header")
	fewCols := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,1\n"
	_, err = ts.LoadFromCSV(strings.NewReader(fewCols))
	require.Error(t, err, "want error for too few columns")
	badBat := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,0,0,abc,0.1\n"
	_, err = ts.LoadFromCSV(strings.NewReader(badBat))
	require.Error(t, err, "want error for bad bat_score")
}

func TestLoadFromDB_InvalidArgsAndSuccess(t *testing.T) {
	t.Parallel()
	_, err := ts.LoadFromDB(context.Background(), nil, 1, "T20", "2019")
	require.Error(t, err, "want error for nil repo")
	repo := &fakeRepo2{}
	_, err = ts.LoadFromDB(context.Background(), repo, 0, "T20", "2019")
	require.Error(t, err, "want error for invalid match id")
	_, err = ts.LoadFromDB(context.Background(), repo, 1, "", "2019")
	require.Error(t, err, "want error for empty format")
	_, err = ts.LoadFromDB(context.Background(), repo, 1, "T20", "")
	require.Error(t, err, "want error for empty season")
	repo.players = []db.PoolPlayer{
		{Name: "A", IsBowler: true, BatScore: 0.3, BowlScore: 0.7},
		{Name: "K", IsKeeper: true, BatScore: 0.5, BowlScore: 0.2},
	}
	ps, err := ts.LoadFromDB(context.Background(), repo, 1, "T20", "2019")
	require.NoError(t, err)
	require.Len(t, ps, 2)
	require.Equal(t, "A", ps[0].Name)
	require.True(t, ps[0].IsBowler)
	require.True(t, ps[1].IsKeeper)
}

func TestSelect_NoKeeperAvailable(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := []ts.Player{{Name: "A", BatScore: 0.9}, {Name: "B", BatScore: 0.8}}
	_, err := ts.Select(pool, w, ts.Constraints{Size: 1, RequireKeeper: true})
	require.Error(t, err, "want error when keeper required but none available")
}

func TestSelect_BowlerReplacementFallback(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	// team of size 2, need 1 bowler; only one candidate bowler in rest should replace a non-bowler
	pool := []ts.Player{
		{Name: "A", BatScore: 0.9},
		{Name: "B", BatScore: 0.8},
		{Name: "C", BowlScore: 0.9, IsBowler: true},
	}
	team, err := ts.Select(pool, w, ts.Constraints{Size: 2, MinBowlers: 1})
	require.NoError(t, err)
	require.GreaterOrEqual(t, countBowl(team), 1, "want at least 1 bowler")
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
	_, err := ts.LoadFromDB(context.Background(), repo, 1, "ODI", "2019")
	require.Error(t, err, "want error propagated from repo")
}
